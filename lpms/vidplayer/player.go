package vidplayer

import (
	"context"
	"io"
	"net/http"

	"strings"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/nareix/joy4/av"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

//VidPlayer is the module that handles playing video. For now we only support RTMP and HLS play.
type VidPlayer struct {
	RtmpServer *joy4rtmp.Server
	// httpServer *http
	// demuxer    av.Demuxer
}

//HandleRTMPPlay is the handler when there is a RTMP request for a video. The source should write
//into the MuxCloser. The easiest way is through avutil.Copy.
func (s *VidPlayer) HandleRTMPPlay(getStream func(ctx context.Context, reqPath string, dst av.MuxCloser) error) error {
	s.RtmpServer.HandlePlay = func(conn *joy4rtmp.Conn) {
		glog.V(logger.Info).Infof("LPMS got RTMP request @ %v", conn.URL)

		ctx := context.Background()
		c := make(chan error, 1)
		go func() { c <- getStream(ctx, conn.URL.Path, conn) }()
		select {
		case err := <-c:
			glog.V(logger.Error).Infof("Rtmp getStream Error: %v", err)
		}
		//TODO: test if code ever gets here
	}
	return nil
}

//HandleHTTPPlay is the handler when there is a HLA request. The source should write the raw bytes into the io.Writer,
//for either the playlist or the segment.
func (s *VidPlayer) HandleHTTPPlay(getStream func(ctx context.Context, reqPath string, writer io.Writer) error) error {
	http.HandleFunc("/stream/", func(w http.ResponseWriter, r *http.Request) {
		glog.V(logger.Info).Infof("LPMS got HTTP request @ %v", r.URL.Path)

		if !strings.HasSuffix(r.URL.Path, ".m3u8") && !strings.HasSuffix(r.URL.Path, ".ts") {
			http.Error(w, "LPMS only accepts HLS requests over HTTP (m3u8, ts).", 500)
		}

		ctx := context.Background()
		c := make(chan error, 1)
		go func() { c <- getStream(ctx, r.URL.Path, w) }()
		select {
		case err := <-c:
			glog.V(logger.Error).Infof("Http get Stream Error: %v", err)
		}
	})
	return nil
}
