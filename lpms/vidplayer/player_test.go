package vidplayer

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/av/avutil"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

func TestRTMP(t *testing.T) {
	server := &joy4rtmp.Server{Addr: ":1936"}
	player := &VidPlayer{rtmpServer: server}
	var demuxer av.Demuxer
	gotUpvid := false
	gotPlayvid := false
	player.rtmpServer.HandlePublish = func(conn *joy4rtmp.Conn) {
		gotUpvid = true
		demuxer = conn
	}

	player.HandleRTMPPlay(func(ctx context.Context, reqPath string, dst av.MuxCloser) error {
		gotPlayvid = true
		fmt.Println(reqPath)
		avutil.CopyFile(dst, demuxer)
		return nil
	})

	go server.ListenAndServe()

	ffmpegCmd := "ffmpeg"
	ffmpegArgs := []string{"-re", "-i", "../data/bunny2.mp4", "-c", "copy", "-f", "flv", "rtmp://localhost:1936/movie/stream"}
	go exec.Command(ffmpegCmd, ffmpegArgs...).Run()

	time.Sleep(time.Second * 1)

	if gotUpvid == false {
		t.Fatal("Didn't get the upstream video")
	}

	ffplayCmd := "ffplay"
	ffplayArgs := []string{"rtmp://localhost:1936/movie/stream"}
	go exec.Command(ffplayCmd, ffplayArgs...).Run()

	time.Sleep(time.Second * 1)
	if gotPlayvid == false {
		t.Fatal("Didn't get the downstream video")
	}
}

func TestHLS(t *testing.T) {
	// server := &joy4rtmp.Server{Addr: ":1936"}
	player := &VidPlayer{}
	player.HandleHTTPPlay(func(ctx context.Context, reqPath string, writer io.Writer) error {
		// stream := streamDB.findStream(reqPath)
		// writer.Write(stream.Pop())
		return nil
	})
	// http.HandleFunc("/stream/", func(w http.ResponseWriter, r *http.Request) {
	// })
}
