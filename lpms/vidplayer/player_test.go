package vidplayer

import (
	"context"
	"fmt"
	"io"
	"testing"

	"strings"

	"time"

	"github.com/kz26/m3u8"
	lpmsio "github.com/livepeer/go-livepeer/lpms/io"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/av/avutil"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

func TestRTMP(t *testing.T) {
	server := &joy4rtmp.Server{Addr: ":1936"}
	player := &VidPlayer{RtmpServer: server}
	var demuxer av.Demuxer
	gotUpvid := false
	gotPlayvid := false
	player.RtmpServer.HandlePublish = func(conn *joy4rtmp.Conn) {
		gotUpvid = true
		demuxer = conn
	}

	player.HandleRTMPPlay(func(ctx context.Context, reqPath string, dst av.MuxCloser) error {
		gotPlayvid = true
		fmt.Println(reqPath)
		avutil.CopyFile(dst, demuxer)
		return nil
	})

	// go server.ListenAndServe()

	// ffmpegCmd := "ffmpeg"
	// ffmpegArgs := []string{"-re", "-i", "../data/bunny2.mp4", "-c", "copy", "-f", "flv", "rtmp://localhost:1936/movie/stream"}
	// go exec.Command(ffmpegCmd, ffmpegArgs...).Run()

	// time.Sleep(time.Second * 1)

	// if gotUpvid == false {
	// 	t.Fatal("Didn't get the upstream video")
	// }

	// ffplayCmd := "ffplay"
	// ffplayArgs := []string{"rtmp://localhost:1936/movie/stream"}
	// go exec.Command(ffplayCmd, ffplayArgs...).Run()

	// time.Sleep(time.Second * 1)
	// if gotPlayvid == false {
	// 	t.Fatal("Didn't get the downstream video")
	// }
}

func TestHLS(t *testing.T) {
	player := &VidPlayer{}
	stream := lpmsio.NewStream("test")
	stream.HLSTimeout = time.Second * 5
	//Write some packets into the stream
	stream.WriteHLSPlaylistToStream(m3u8.MediaPlaylist{})
	stream.WriteHLSSegmentToStream(lpmsio.HLSSegment{})
	var buffer *lpmsio.HLSBuffer
	player.HandleHTTPPlay(func(ctx context.Context, reqPath string, writer io.Writer) error {
		//if can't find local cache, start downloading, and store in cache.
		if buffer == nil {
			buffer := lpmsio.NewHLSBuffer()
			ec := make(chan error, 1)
			go func() { ec <- stream.ReadHLSFromStream(buffer) }()
			// select {
			// case err := <-ec:
			// 	return err
			// }
		}

		if strings.HasSuffix(reqPath, ".m3u8") {
			pl, err := buffer.WaitAndPopPlaylist(ctx)
			if err != nil {
				return err
			}
			_, err = writer.Write(pl.Encode().Bytes())
			if err != nil {
				return err
			}
			return nil
		}

		if strings.HasSuffix(reqPath, ".ts") {
			pathArr := strings.Split(reqPath, "/")
			segName := pathArr[len(pathArr)-1]
			seg, err := buffer.WaitAndPopSegment(ctx, segName)
			if err != nil {
				return err
			}
			_, err = writer.Write(seg)
			if err != nil {
				return err
			}
		}

		return lpmsio.ErrNotFound
	})

	// go http.ListenAndServe(":8000", nil)

	//TODO: Add tests for checking if packets were written, etc.
}
