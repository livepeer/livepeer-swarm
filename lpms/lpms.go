//The RTMP server.  This will put up a RTMP endpoint when starting up Swarm.
//To integrate with LPMS means your code will become the source / destination of the media server.
//This RTMP endpoint is mainly used for video upload.  The expected url is rtmp://localhost:port/livepeer/stream
package lpms

import (
	"context"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/livepeer/go-livepeer/lpms/io"
	streamingVizClient "github.com/livepeer/streamingviz/client"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

// "github.com/livepeer/go-livepeer/livepeer/network"
// "github.com/livepeer/go-livepeer/livepeer/storage"
// "github.com/livepeer/go-livepeer/livepeer/storage/streaming"

//Any LivepeerNode implements this interface - we want to start moving this direction
//Ideally, LPMS will only call these functions
type VideoSource interface {
	// Broadcast(inStream *av.StreamReader) (streamID string)
	// StartStream(streamID string) (outStream *av.StreamReader, err error)
	StopStream(streamID string) (err error)
}

//LPMS interface
// func Transcode(inStream *av.StreamReader, outStream *av.StreamWriter) (err error) {

// }

type Server struct {
	config *Config
	viz    *streamingVizClient.Client
	// rtmpConn   *joy4rtmp.Conn
	rtmpServer *joy4rtmp.Server
}

//Start new HTTP & RTMP server.
//The HTTP server will serve the local player.
//The RTMP server will listen for incoming stream.
// func NewVideoServer(rtmpPort string, httpPort string, livepeerNode LivepeerNode) (s *Server, err error) {
// func NewVideoServer(c *Config, videoSource VideoSource) (s *Server, err error) {
func NewVideoServer(c *Config) (s *Server, err error) {
	context.TODO()
	server := &Server{config: c, rtmpServer: &joy4rtmp.Server{Addr: ":" + c.RtmpPort}}
	return server, nil
}

// func NewVideoServer(config *Config) {
// }

// func AttachHttpServer(s http) {
// }

//HandleRtmpPlay handles playing a RTMP video.  Should load a stream and play it.
func (s *Server) HandleRtmpPlay(f func(ctx context.Context, reqPath string, dst io.Stream) error) error {
	s.rtmpServer.HandlePlay = func(conn *joy4rtmp.Conn) {
		glog.V(logger.Info).Infof("LPMS got RTMP request @ %v", conn.URL)

		dst := &io.LPMSStream{RtmpOutput: conn}
		ctx := context.Background()
		c := make(chan error, 1)
		go func() { c <- f(ctx, conn.URL.Path, dst) }()
		select {
		case err := <-c:
			glog.V(logger.Error).Infof("RtmpPlay Error: %v", err)
		}
	}
	return nil
}

func (s *Server) HandlePlay(ctx context.Context, f func(streamID string) (io.Stream, error)) error {
	// self.handlePlay(ctx, self.rtmpConn, f)
	return nil
}

func (self *Server) HandleNewStream(f func() (streamID string)) error {
	return nil
}

// func (self *Server) handlePlay(ctx context.Context, conn *joy4rtmp.Conn, getStream func(StreamID string) (io.Stream, error)) {
// 	glog.V(logger.Info).Infof("Trying to play stream at %v", conn.URL)

// 	// Parse the streamID from the path host:port/stream/{streamID}
// 	var strmID string
// 	regex, _ := regexp.Compile("\\/stream\\/([[:alpha:]]|\\d)*")
// 	match := regex.FindString(conn.URL.Path)
// 	if match != "" {
// 		strmID = strings.Replace(match, "/stream/", "", -1)
// 	}

// 	glog.V(logger.Info).Infof("Got streamID as %v", strmID)
// 	self.viz.LogConsume(strmID)
// 	stream, err := getStream(strmID)
// 	// stream, err := getStream(ctx, strmID)
// 	// ctx.Timeout

// 	// stream, err := streamer.GetStreamByStreamID(streaming.StreamID(strmID))
// 	if stream == nil {
// 		// stream, err = streamer.SubscribeToStream(strmID)
// 		// if err != nil {
// 		glog.V(logger.Info).Infof("Error subscribing to stream %v", err)
// 		// 	return
// 		// }
// 	} else {
// 		fmt.Println("Found stream: ", strmID)
// 	}

// 	//Send subscribe request
// 	// forwarder.Stream(strmID, kademlia.Address(common.HexToHash("")))

// 	//Copy chunks to outgoing connection
// 	go io.CopyRTMPFromStreamNew(ctx, conn, stream)
// }

// func (self *Server) startRtmpServer(c *Config) {
// 	self.rtmpServer.HandlePlay = func(conn *joy4rtmp.Conn) {
// 		self.rtmpConn = conn
// 	}
// }

//You already have http/rtmp servers, just want to add LPMS handlers

// func StartVideoServer(rtmpPort string, httpPort string, srsRtmpPort string, srsHttpPort string, streamer *streaming.Streamer,
// 	forwarder storage.CloudStore, streamdb *network.StreamDB, viz *streamingVizClient.Client) {

// 	common.SetConfig(srsRtmpPort, srsHttpPort, rtmpPort, httpPort)
// 	server.StartRTMPServer(rtmpPort, srsRtmpPort, srsHttpPort, streamer, forwarder, viz)
// 	server.StartHTTPServer(rtmpPort, httpPort, srsRtmpPort, srsHttpPort, streamer, forwarder, streamdb, viz)

// }
