//Adding the RTMP server.  This will put up a RTMP endpoint when starting up Swarm.
//It's a simple RTMP server that will take a video stream and play it right back out.
//After bringing up the Swarm node with RTMP enabled, try it out using:
//
//ffmpeg -re -i bunny.mp4 -c copy -f flv rtmp://localhost/movie
//ffplay rtmp://localhost/movie

package lpms

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/ethereum/go-ethereum/swarm/network/kademlia"
	"github.com/livepeer/go-livepeer/livepeer/network"
	"github.com/livepeer/go-livepeer/livepeer/storage"
	"github.com/livepeer/go-livepeer/livepeer/storage/streaming"
	"github.com/livepeer/go-livepeer/lpms/common"
	"github.com/nareix/joy4/av"

	"github.com/livepeer/go-livepeer/lpms/server"
	newLPMS "github.com/livepeer/lpms"
	newStream "github.com/livepeer/lpms/stream"
	streamingVizClient "github.com/livepeer/streamingviz/client"
)

func StartVideoServer(rtmpPort string, httpPort string, srsRtmpPort string, srsHttpPort string, streamer *streaming.Streamer,
	forwarder storage.CloudStore, streamdb *network.StreamDB, viz *streamingVizClient.Client) {

	common.SetConfig(srsRtmpPort, srsHttpPort, rtmpPort, httpPort)
	server.StartRTMPServer(rtmpPort, srsRtmpPort, srsHttpPort, streamer, forwarder, viz)
	server.StartHTTPServer(rtmpPort, httpPort, srsRtmpPort, srsHttpPort, streamer, forwarder, streamdb, viz)

}

func StartLPMS(rtmpPort string, httpPort string, srsRtmpPort string, srsHttpPort string, streamer *streaming.Streamer,
	forwarder storage.CloudStore, streamdb *network.StreamDB, viz *streamingVizClient.Client) {

	server := newLPMS.New(rtmpPort, httpPort, srsRtmpPort, srsHttpPort)
	var strmID string
	var stream *streaming.Stream
	var lpmsStream newStream.Stream
	var ctx context.Context
	var cancel context.CancelFunc

	server.HandleRTMPPublish(
		//getStreamID
		func(reqPath string) (string, error) {
			ctx, cancel = context.WithCancel(context.Background())
			return "", nil
		},
		//getStream
		func(reqPath string) (newStream.Stream, error) {
			regex, _ := regexp.Compile("\\/stream\\/([[:alpha:]]|\\d)*")
			match := regex.FindString(reqPath)
			if match != "" {
				strmID = strings.Replace(match, "/stream/", "", -1)
				stream, _ = streamer.GetStreamByStreamID(streaming.StreamID(strmID))
			}

			if stream == nil {
				stream, _ = streamer.AddNewStream()
				glog.V(logger.Info).Infof("Added a new stream with id: %v", stream.ID)
			} else {
				glog.V(logger.Info).Infof("Got streamID as %v", strmID)
			}

			lpmsStream = newStream.NewVideoStream(strmID)

			viz.LogBroadcast(string(stream.ID))
			go lpmsStream.ReadRTMPFromStream(ctx, stream)

			return lpmsStream, nil
		},
		//finishStream
		func(reqPath string) {
			glog.V(logger.Info).Infof("Finish Stream - canceling stream (need to implement handler for Done())")
			cancel()
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

			glog.V(logger.Info).Infof("Got streamID as %v", strmID)
			viz.LogConsume(strmID)
			stream, err := streamer.GetStreamByStreamID(streaming.StreamID(strmID))
			if stream == nil {
				stream, err = streamer.SubscribeToStream(strmID)
				if err != nil {
					glog.V(logger.Info).Infof("Error subscribing to stream %v", err)
					return errors.New("Error subscribing to stream")
				}
			} else {
				fmt.Println("Found stream: ", strmID)
			}

			//Send subscribe request
			forwarder.Stream(strmID, kademlia.Address(ethCommon.HexToHash("")))

			ec := make(chan error, 1)
			go func() {
				ec <- func() error {
					chunk := <-stream.DstVideoChan
					glog.V(logger.Info).Infof("Writing RTMP to player")

					if err := dst.WriteHeader(chunk.HeaderStreams); err != nil {
						glog.V(logger.Error).Infof("Error writing header copying from channel")
						return err
					}

					kfCount := 0

					for {
						select {
						case chunk := <-stream.DstVideoChan:
							if chunk.ID == streaming.EOFStreamMsgID {
								glog.V(logger.Info).Infof("Copying EOF from channel")

								err := dst.WriteTrailer()
								if err != nil {
									glog.V(logger.Error).Infof("Error writing trailer: ", err)
									return err
								}
							}
							if chunk.Packet.IsKeyFrame {
								kfCount = kfCount + 1
							}

							//Wait for the first keyframe
							if kfCount < 2 {
								break
							}

							err := dst.WritePacket(chunk.Packet)
							if err != nil {
								glog.V(logger.Error).Infof("Error writing packet to video player: %s", err)
								forwarder.StopStream(strmID, kademlia.Address(ethCommon.HexToHash("")))
								return err
							}
						}
					}
				}()
			}()

			select {
			case err := <-ec:
				return err
			}

		})

	server.Start()
}
