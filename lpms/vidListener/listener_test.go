package vidlistener

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/nareix/joy4/av"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

func TestError(t *testing.T) {
	server := &joy4rtmp.Server{Addr: ":1937"}
	listener := &VidListener{RtmpServer: server, Streams: make(map[string]LocalStream)}
	listener.HandleRTMPPublish(
		func(streamID chan<- string) error {
			streamID <- "test"
			return nil
		},
		func(ctx context.Context, reqPath string, demux av.DemuxCloser) error {
			// return errors.New("Some Error")
			return nil
		})

	ffmpegCmd := "ffmpeg"
	ffmpegArgs := []string{"-re", "-i", "../data/bunny2.mp4", "-c", "copy", "-f", "flv", "rtmp://localhost:1937/movie/stream"}
	go exec.Command(ffmpegCmd, ffmpegArgs...).Run()

	go listener.RtmpServer.ListenAndServe()

	time.Sleep(time.Second * 1)
	// fmt.Println(listener.Streams)
	// if listener.SetStream == false {
	// 	t.Fatal("Didn't set stream")
	// }

	// if stream := listener.Stream; stream.StreamID != "test" {
	// 	t.Fatal("Should have exited before finishing (so the stream should be removed)")
	// }
}

func TestRTMPWithServer(t *testing.T) {
	server := &joy4rtmp.Server{Addr: ":1936"}
	listener := &VidListener{RtmpServer: server, Streams: make(map[string]LocalStream)}
	listener.HandleRTMPPublish(
		func(reqPath string, streamID chan<- string) error {
			streamID <- "teststream"
			return nil
		},
		func(ctx context.Context, reqPath string, demux av.DemuxCloser) error {
			header, err := demux.Streams()
			if err != nil {
				t.Fatal("Failed ot read stream header")
			}
			fmt.Println("header: ", header)

			counter := 0
			fmt.Println("data: ")
			for {
				packet, err := demux.ReadPacket()
				if err != nil {
					t.Fatal("Failed to read packets")
				}
				fmt.Print("\r", len(packet.Data))
				counter = counter + 1
			}
		})
	ffmpegCmd := "ffmpeg"
	ffmpegArgs := []string{"-re", "-i", "../data/bunny2.mp4", "-c", "copy", "-f", "flv", "rtmp://localhost:1936/movie/stream"}
	go exec.Command(ffmpegCmd, ffmpegArgs...).Run()

	go listener.RtmpServer.ListenAndServe()

	time.Sleep(time.Second * 1)
	if stream := listener.Streams["teststream"]; stream.StreamID != "teststream" {
		t.Fatal("Server did not set stream")
	}

	time.Sleep(time.Second * 1)
}
