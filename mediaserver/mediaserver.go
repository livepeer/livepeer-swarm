//mediaserver is the place we set up the handlers for network requests.

package mediaserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ericxtang/m3u8"
	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/ethereum/go-ethereum/swarm/network/kademlia"
	"github.com/livepeer/go-livepeer/livepeer/network"
	"github.com/livepeer/go-livepeer/livepeer/storage"
	"github.com/livepeer/go-livepeer/livepeer/streaming"

	"net/url"

	lpmscore "github.com/livepeer/lpms/core"
	lpmsStream "github.com/livepeer/lpms/stream"
	streamingVizClient "github.com/livepeer/streamingviz/client"
	"github.com/nareix/joy4/av/pubsub"
)

var ErrNotFound = errors.New("NotFound")
var ErrStreamPublish = errors.New("StreamPublishError")
var ErrHLSPlay = errors.New("ErrHLSPlay")
var HLSWaitTime = time.Second * 10
var HLSBufferCap = uint(43200) //12 hrs assuming 1s segment
var HLSBufferWindow = uint(5)
var HLSUnsubscribeWaitLimit = time.Second * 20

func startHlsUnsubscribeWorker(hlsSubTimer map[streaming.StreamID]time.Time, streamer *streaming.Streamer, forwarder storage.CloudStore, limit time.Duration) {
	for {
		time.Sleep(time.Second * 5)
		for sid, t := range hlsSubTimer {
			if time.Since(t) > limit {
				streamer.UnsubscribeToHLSStream(sid.String(), "local")
				forwarder.StopStream(sid.String(), kademlia.Address(ethCommon.HexToHash("")), lpmsStream.HLS) //This could fail if it's a local stream, but it's ok.
				delete(hlsSubTimer, sid)
			}
		}
	}
}

func StartLPMS(rtmpPort string, httpPort string, streamer *streaming.Streamer, forwarder storage.CloudStore, streamdb *network.StreamDB,
	viz *streamingVizClient.Client, hive *network.Hive, ffmpegPath string, vodPath string) {

	hlsSubTimer := make(map[streaming.StreamID]time.Time)
	go startHlsUnsubscribeWorker(hlsSubTimer, streamer, forwarder, HLSUnsubscribeWaitLimit)

	server := lpmscore.New(rtmpPort, httpPort, ffmpegPath, vodPath)

	server.HandleHLSPlay(
		//getMasterPlaylist
		func(url *url.URL) (*m3u8.MasterPlaylist, error) {
			return nil, nil
		},
		//getMediaPlaylist
		func(url *url.URL) (*m3u8.MediaPlaylist, error) {
			strmID := parseStreamID(url.Path)

			//Validate the stream ID format
			sid := streaming.StreamID(strmID)
			nodeID, streamID := sid.SplitComponents()

			if strmID == "" || streamID == "" {
				glog.Errorf("Cannot find stream for %v", url.Path)
				return nil, errors.New("Stream Not Found")
			}

			strm := streamer.GetNetworkStream(streaming.StreamID(strmID))
			if strm == nil {
				if streamer.SelfAddress != nodeID {
					glog.Infof("Cannot find HLS stream:%v locally, forwarding request to the newtork", strmID)
					forwarder.Stream(strmID, kademlia.Address(ethCommon.HexToHash("")), lpmsStream.HLS)
				} else {
					glog.Infof("Cannot find HLS stream:%v, returning 404", strmID)
					return nil, ErrNotFound
				}
			} else {
				// glog.Infof("Found HLS stream:%v locally", strmID)
			}

			subID := "local"
			hlsBuffer := streamer.GetHLSMuxer(strmID, subID)
			if hlsBuffer == nil {
				glog.Infof("Creating new HLS buffer")
				hlsBuffer = lpmsStream.NewHLSBuffer(HLSBufferWindow, HLSBufferCap)
				err := streamer.SubscribeToHLSStream(strmID, subID, hlsBuffer)
				if err != nil {
					glog.Errorf("Error subscribing to hls stream:%v", url.Path)
					return nil, err
				}
			} else {
				// glog.Infof("Found HLS buffer for %v", reqPath)
			}
			// glog.Infof("Buffer subscribed to local stream:%v ", strmID)

			startTime := time.Now()
			for {
				// _, err := hlsBuffer.(*lpmsStream.HLSBuffer).GeneratePlaylist(0)
				_, err := hlsBuffer.(*lpmsStream.HLSBuffer).LatestPlaylist()
				if err == nil {
					hlsSubTimer[streaming.StreamID(strmID)] = time.Now()
					return hlsBuffer.(*lpmsStream.HLSBuffer).LatestPlaylist()
				} else {
					glog.Errorf("Error generating pl: %v", err)
				}
				time.Sleep(time.Second * 2) //Sleep for 2 seconds so the segments start to get to the buffer
				if time.Since(startTime) > HLSWaitTime {
					return nil, ErrNotFound
				}
			}
		},
		//getSegment
		func(url *url.URL) ([]byte, error) {
			strmID := parseStreamID(url.Path)
			buftmp := streamer.GetHLSMuxer(strmID, "local")
			if buftmp == nil {
				return nil, ErrNotFound
			}

			buf, ok := buftmp.(*lpmsStream.HLSBuffer)
			if !ok {
				return nil, ErrHLSPlay
			}
			sn := parseSegName(url.Path)
			return buf.WaitAndPopSegment(context.Background(), sn)
		})

	server.HandleRTMPPublish(
		//makeStreamID
		func(url *url.URL) (strmID string) {
			rtmpStrmID := streaming.StreamID(parseStreamID(url.Path))
			if rtmpStrmID == "" {
				rtmpStrmID = streaming.MakeStreamID(streamer.SelfAddress, fmt.Sprintf("%x", streaming.RandomStreamID()))
			}
			return rtmpStrmID.String()
		},
		//gotStream
		func(url *url.URL, rtmpStrm *lpmsStream.VideoStream) (err error) {
			rtmpStrmID := streaming.StreamID(rtmpStrm.GetStreamID())
			nodeID, _ := rtmpStrmID.SplitComponents()
			if nodeID != streamer.SelfAddress {
				glog.Errorf("Invalid rtmp strmID - nodeID component needs to be self.")
				return ErrStreamPublish
			}

			rtmpStream := streamer.GetNetworkStream(rtmpStrmID)
			if rtmpStream == nil {
				var rtmpErr error
				rtmpStream, rtmpErr = streamer.AddNewNetworkStream(rtmpStrmID, lpmsStream.RTMP)
				if rtmpErr != nil {
					glog.Errorf("Error when creating RTMP stream: %v", rtmpErr)
					return ErrStreamPublish
				}
			}

			hlsStrmID := streaming.StreamID(url.Query().Get("hlsStrmID"))
			if hlsStrmID == "" {
				hlsStrmID = streaming.MakeStreamID(streamer.SelfAddress, fmt.Sprintf("%x", streaming.RandomStreamID()))
			}
			nodeID, _ = hlsStrmID.SplitComponents()
			if nodeID != streamer.SelfAddress {
				glog.Errorf("Invalid hlsStrmID - nodeID component needs to be self.")
				return ErrStreamPublish
			}
			hlsStream, err := streamer.AddNewNetworkStream(hlsStrmID, lpmsStream.HLS)
			if err != nil {
				glog.Errorf("Error when creating HLS stream: %v", err)
				return ErrStreamPublish
			}

			glog.Infof("RTMP streamID is %v", rtmpStream.GetStreamID())
			glog.Infof("HLS streamID is %v", hlsStream.GetStreamID())

			viz.LogBroadcast(rtmpStream.GetStreamID())
			viz.LogBroadcast(hlsStream.GetStreamID())
			return nil
		},
		//endStream
		func(url *url.URL, rtmpStrm *lpmsStream.VideoStream) error {
			glog.Infof("Finish Stream - canceling stream (need to implement handler for Done())")
			streamer.DeleteNetworkStream(streaming.StreamID(rtmpStrm.GetStreamID()))
			streamer.UnsubscribeAll(rtmpStrm.GetStreamID())

			// streamer.DeleteNetworkStream(streaming.StreamID(hlsStrmID))
			// streamer.UnsubscribeAll(hlsStrmID)
			return nil
		})

	server.HandleRTMPPlay(
		//getStream
		func(url *url.URL) (lpmsStream.Stream, error) {
			glog.Infof("Got req: ", url.Path)

			var strmID string
			regex, _ := regexp.Compile("\\/stream\\/([[:alpha:]]|\\d)*")
			match := regex.FindString(url.Path)
			if match != "" {
				strmID = strings.Replace(match, "/stream/", "", -1)
			}

			if strmID == "" {
				glog.Errorf("Cannot find stream for %v", url.Path)
				return nil, errors.New("Stream Not Found")
			}

			// glog.Infof("Got RTMP streamID as %v", strmID)
			viz.LogConsume(strmID)

			strm := streamer.GetNetworkStream(streaming.StreamID(strmID))
			if strm == nil {
				//Send subscribe request
				glog.Infof("No local RTMP stream found - forwarding request to the network")
				forwarder.Stream(strmID, kademlia.Address(ethCommon.HexToHash("")), lpmsStream.RTMP)
			}
			q := pubsub.NewQueue()
			subID := streaming.RandomStreamID().Str()

			err := streamer.SubscribeToRTMPStream(strmID, subID, q)
			if err != nil {
				glog.Errorf("Error subscribing to stream %v", err)
				return nil, err
			}

			return strm, nil
		})

	http.HandleFunc("/createStream", func(w http.ResponseWriter, r *http.Request) {
		strmID := streaming.MakeStreamID(streamer.SelfAddress, fmt.Sprintf("%x", streaming.RandomStreamID()))
		newRTMPStream, _ := streamer.AddNewNetworkStream(strmID, lpmsStream.RTMP)
		res := map[string]string{"streamID": newRTMPStream.GetStreamID()}

		js, err := json.Marshal(res)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		glog.Infof("Created Stream: %s", js)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})

	http.HandleFunc("/localStreams", func(w http.ResponseWriter, r *http.Request) {
		streams := streamer.GetAllNetworkStreams()
		ret := make([]map[string]string, 0, len(streams))
		for _, s := range streamer.GetAllNetworkStreams() {
			sid := streaming.StreamID(s.GetStreamID())
			nodeID, _ := sid.SplitComponents()
			var source string

			if nodeID == streamer.SelfAddress {
				source = "local"
			} else {
				source = fmt.Sprintf("%v", nodeID)
			}

			if s.Format == lpmsStream.HLS {
				ret = append(ret, map[string]string{"format": "hls", "streamID": s.GetStreamID(), "source": source})
			} else {
				ret = append(ret, map[string]string{"format": "rtmp", "streamID": s.GetStreamID(), "source": source})
			}
		}

		js, err := json.Marshal(ret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})

	http.HandleFunc("/peersCount", func(w http.ResponseWriter, r *http.Request) {
		c := hive.PeersCount()
		ret := make(map[string]int)
		ret["count"] = c

		js, err := json.Marshal(ret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})

	http.HandleFunc("/streamerStatus", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(streamer.CurrentStatus()))
	})

	fs := http.FileServer(http.Dir("static"))
	fmt.Println("Serving static files from: ", fs)
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/broadcast.html", 301)
	})

	server.Start(context.Background())
}

func parseStreamID(reqPath string) string {
	var strmID string
	regex, _ := regexp.Compile("\\/stream\\/([[:alpha:]]|\\d)*")
	match := regex.FindString(reqPath)
	if match != "" {
		strmID = strings.Replace(match, "/stream/", "", -1)
	}
	return strmID
}

func parseSegName(reqPath string) string {
	var segName string
	regex, _ := regexp.Compile("\\/stream\\/.*\\.ts")
	match := regex.FindString(reqPath)
	if match != "" {
		segName = strings.Replace(match, "/stream/", "", -1)
	}
	return segName
}
