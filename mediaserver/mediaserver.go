//Adding the RTMP server.  This will put up a RTMP endpoint when starting up Swarm.
//It's a simple RTMP server that will take a video stream and play it right back out.
//After bringing up the Swarm node with RTMP enabled, try it out using:
//
//ffmpeg -re -i bunny.mp4 -c copy -f flv rtmp://localhost/movie
//ffplay rtmp://localhost/movie

package mediaserver

import (
	"context"
	"errors"
	"regexp"
	"strings"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger"
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

// func StartVideoServer(rtmpPort string, httpPort string, srsRtmpPort string, srsHttpPort string, streamer *streaming.Streamer,
// 	forwarder storage.CloudStore, streamdb *network.StreamDB, viz *streamingVizClient.Client) {

// common.SetConfig(srsRtmpPort, srsHttpPort, rtmpPort, httpPort)
// server.StartRTMPServer(rtmpPort, srsRtmpPort, srsHttpPort, streamer, forwarder, viz)
// server.StartHTTPServer(rtmpPort, httpPort, srsRtmpPort, srsHttpPort, streamer, forwarder, streamdb, viz)

// }

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
			// ctx, cancel := context.WithCancel(context.Background())
			return "", nil
		},
		//getStream
		func(reqPath string) (lpmsStream.Stream, lpmsStream.Stream, error) {
			// if localRTMPStream == nil {
			// 	localRTMPStream, _ = streamer.AddNewStream()
			// 	localHLSBuffer = lpmsStream.NewHLSBuffer()
			// 	glog.V(logger.Info).Infof("Added a new stream with id: %v", localRTMPStream.ID)
			// } else {
			// 	glog.V(logger.Info).Infof("Got streamID as %v", localRTMPStream.ID)
			// }

			// newRTMPStream = lpmsStream.NewVideoStream(localRTMPStream.ID.String())
			// newHLSStream = lpmsStream.NewVideoStream(localRTMPStream.ID.String())
			newRTMPStream, _ := streamer.AddNewNetworkStream()
			newHLSStream, _ := streamer.AddNewNetworkStream()
			glog.Infof("RTMP streamID is %v", newRTMPStream.GetStreamID())
			glog.Infof("HLS streamID is %v", newHLSStream.GetStreamID())

			viz.LogBroadcast(newRTMPStream.GetStreamID())
			viz.LogBroadcast(newHLSStream.GetStreamID())
			// go newRTMPStream.ReadRTMPFromStream(ctx, localRTMPStream)
			// go newHLSStream.ReadHLSFromStream(localHLSBuffer)

			return newRTMPStream, newHLSStream, nil
		},
		//finishStream
		func(reqPath string) {
			glog.V(logger.Info).Infof("Finish Stream - canceling stream (need to implement handler for Done())")
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

			return avutil.CopyFile(dst, q.Oldest())
		})

	server.Start()
}

// func StreamChanToDst(src chan *streaming.VideoChunk, dst av.MuxCloser) error {
// 	chunk := <-src

// 	if err := dst.WriteHeader(chunk.HeaderStreams); err != nil {
// 		glog.V(logger.Error).Infof("Error writing header copying from channel")
// 		return err
// 	}

// 	kfCount := 0

// 	for {
// 		select {
// 		case chunk := <-src:
// 			if chunk.ID == streaming.EOFStreamMsgID {
// 				glog.V(logger.Info).Infof("Copying EOF from channel")

// 				err := dst.WriteTrailer()
// 				if err != nil {
// 					glog.V(logger.Error).Infof("Error writing trailer: ", err)
// 					return err
// 				}
// 			}
// 			if chunk.Packet.IsKeyFrame {
// 				kfCount = kfCount + 1
// 			}

// 			//Wait for the first keyframe
// 			if kfCount < 2 {
// 				break
// 			}

// 			err := dst.WritePacket(chunk.Packet)
// 			if err != nil {
// 				glog.V(logger.Error).Infof("Error writing packet to video player: %s", err)
// 				return err
// 			}
// 		}
// 	}
// }
