//mediaserver is the place we set up the handlers for network requests.

package mediaserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/ethereum/go-ethereum/swarm/network/kademlia"
	"github.com/livepeer/go-livepeer/livepeer/network"
	"github.com/livepeer/go-livepeer/livepeer/storage"
	"github.com/livepeer/go-livepeer/livepeer/streaming"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/av/avutil"

	"github.com/livepeer/lpms"
	lpmsStream "github.com/livepeer/lpms/stream"
	streamingVizClient "github.com/livepeer/streamingviz/client"
	"github.com/nareix/joy4/av/pubsub"
)

func StartLPMS(rtmpPort string, httpPort string, srsRtmpPort string, srsHttpPort string, streamer *streaming.Streamer,
	forwarder storage.CloudStore, streamdb *network.StreamDB, viz *streamingVizClient.Client) {

	server := lpms.New(rtmpPort, httpPort, srsRtmpPort, srsHttpPort)

	server.HandleHLSPlay(
		func(reqPath string) (*lpmsStream.HLSBuffer, error) {
			var strmID string
			regex, _ := regexp.Compile("\\/stream\\/([[:alpha:]]|\\d)*")
			match := regex.FindString(reqPath)
			if match != "" {
				strmID = strings.Replace(match, "/stream/", "", -1)
			}

			if strmID == "" {
				glog.Errorf("Cannot find stream for %v", reqPath)
				return nil, errors.New("Stream Not Found")
			}

			strm := streamer.GetNetworkStream(streaming.StreamID(strmID))
			if strm == nil {
				glog.Infof("Cannot find HLS stream:%v locally, forwarding request to the newtork", strmID)
				forwarder.Stream(strmID, kademlia.Address(ethCommon.HexToHash("")), lpmsStream.HLS)
			} else {
				glog.Infof("Found HLS stream:%v locally", strmID)
			}

			hlsBuffer := streamer.GetHLSMuxer(strmID)
			if hlsBuffer == nil {
				glog.Infof("Creating new HLS buffer")
				hlsBuffer = lpmsStream.NewHLSBuffer()
				ctx := context.Background()
				err := streamer.SubscribeToHLSStream(ctx, strmID, "local", hlsBuffer)
				if err != nil {
					glog.Errorf("Error subscribing to hls stream:%v", reqPath)
					return nil, err
				}
			}
			glog.Infof("Buffer subscribed to local stream:%v ", strmID)

			return hlsBuffer.(*lpmsStream.HLSBuffer), nil
		})

	server.HandleRTMPPublish(
		//getStreamID
		func(reqPath string) (string, error) {
			return "", nil
		},
		//getStream
		func(reqPath string) (lpmsStream.Stream, lpmsStream.Stream, error) {

			newRTMPStream, _ := streamer.AddNewNetworkStream()
			newHLSStream, _ := streamer.AddNewNetworkStream()
			glog.Infof("RTMP streamID is %v", newRTMPStream.GetStreamID())
			glog.Infof("HLS streamID is %v", newHLSStream.GetStreamID())

			viz.LogBroadcast(newRTMPStream.GetStreamID())
			viz.LogBroadcast(newHLSStream.GetStreamID())

			return newRTMPStream, newHLSStream, nil
		},
		//finishStream
		func(reqPath string) {
			glog.Infof("Finish Stream - canceling stream (need to implement handler for Done())")
		})

	server.HandleRTMPPlay(
		//getStream
		func(ctx context.Context, reqPath string, dst av.MuxCloser) error {
			glog.Infof("Got req: ", reqPath)

			var strmID string
			regex, _ := regexp.Compile("\\/stream\\/([[:alpha:]]|\\d)*")
			match := regex.FindString(reqPath)
			if match != "" {
				strmID = strings.Replace(match, "/stream/", "", -1)
			}

			if strmID == "" {
				glog.Errorf("Cannot find stream for %v", reqPath)
				return errors.New("Stream Not Found")
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
			err := streamer.SubscribeToRTMPStream(ctx, strmID, "local", q)
			if err != nil {
				glog.Errorf("Error subscribing to stream %v", err)
				return err
			}

			ec := make(chan error)
			go func() { ec <- avutil.CopyFile(dst, q.Oldest()) }()
			select {
			case err := <-ec:
				glog.Errorf("Error copying to local player: %v", err)
				forwarder.StopStream(strmID, kademlia.Address(ethCommon.HexToHash("")), lpmsStream.RTMP)
				return err
			}
		})

	fs := http.FileServer(http.Dir("static"))
	fmt.Println("Serving static files from: ", fs)
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/broadcast.html", 301)
	})

	server.Start()
}
