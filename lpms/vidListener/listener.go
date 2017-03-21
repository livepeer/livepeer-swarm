package vidlistener

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/nareix/joy4/av"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

type LocalStream struct {
	StreamID  string
	Timestamp int64
}

type VidListener struct {
	RtmpServer *joy4rtmp.Server
	Streams    map[string]LocalStream //TODO: This map needs to be synchronous
}

func (s *VidListener) HandleRTMPPublish(
	getStreamID func(reqPath string, streamID chan<- string) error,
	stream func(ctx context.Context, reqPath string, demux av.DemuxCloser) error) error {

	s.RtmpServer.HandlePublish = func(conn *joy4rtmp.Conn) {
		glog.V(logger.Error).Infof("Got RTMP Upstream")

		c := make(chan error, 1)
		streamIDChan := make(chan string, 1)
		var streamID string
		go func() { c <- getStreamID(conn.URL.Path, streamIDChan) }()
		select {
		case id := <-streamIDChan:
			streamID = id
			stream := LocalStream{StreamID: id, Timestamp: time.Now().UTC().UnixNano()}
			s.Streams[id] = stream
			glog.V(logger.Info).Infof("RTMP Stream Created: %v", id)
		case err := <-c:
			glog.V(logger.Error).Infof("RTMP Stream Publish Error: %v", err)
			return
		}

		c = make(chan error, 1)
		go func() { c <- stream(context.Background(), conn.URL.Path, conn) }()
		select {
		case err := <-c:
			glog.V(logger.Error).Infof("RTMP Stream Publish Error: %v", err)
			delete(s.Streams, streamID)
			return
		}
	}
	return nil
}
