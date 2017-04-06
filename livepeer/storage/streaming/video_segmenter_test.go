package streaming

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"time"

	"github.com/kz26/m3u8"
	"github.com/livepeer/lpms"
	"github.com/livepeer/lpms/stream"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/av/avutil"
	"github.com/nareix/joy4/format"
)

type TestStream struct{}

func (s *TestStream) GetStreamID() string { return "test" }
func (s *TestStream) Len() int64          { return 0 }
func (s *TestStream) ReadRTMPFromStream(ctx context.Context, dst av.MuxCloser) error {
	format.RegisterAll()
	wd, _ := os.Getwd()
	file, err := avutil.Open(wd + "/test.flv")
	if err != nil {
		fmt.Println("Error opening file: ", err)
		return err
	}
	header, _ := file.Streams()

	dst.WriteHeader(header)
	for {
		pkt, err := file.ReadPacket()
		if err == io.EOF {
			dst.WriteTrailer()
			return err
		}
		dst.WritePacket(pkt)
	}
}
func (s *TestStream) WriteRTMPToStream(ctx context.Context, src av.DemuxCloser) error { return nil }
func (s *TestStream) WriteHLSPlaylistToStream(pl m3u8.MediaPlaylist) error            { return nil }
func (s *TestStream) WriteHLSSegmentToStream(seg stream.HLSSegment) error             { return nil }
func (s *TestStream) ReadHLSFromStream(buffer stream.HLSMuxer) error                  { return nil }

func TestVideoSegmenter(t *testing.T) {
	workDir := "/Users/erictang/Code/go/src/github.com/livepeer/go-livepeer/livepeer/storage/streaming/tmp"
	os.RemoveAll(workDir)

	strm := &TestStream{}
	url := fmt.Sprintf("rtmp://localhost:%v/stream/%v", "1935", strm.GetStreamID())
	vs := NewFFMpegVideoSegmenter(workDir, strm.GetStreamID(), url)
	server := lpms.New("1935", "", "", "")
	server.HandleRTMPPlay(
		func(ctx context.Context, reqPath string, dst av.MuxCloser) error {
			return strm.ReadRTMPFromStream(ctx, dst)
		})

	go server.Start()

	se := make(chan error, 1)
	opt := SegmenterOptions{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()

	go func() { se <- func() error { return vs.RTMPToHLS(ctx, opt) }() }()
	select {
	case err := <-se:
		if err != context.DeadlineExceeded {
			t.Errorf("Should exceed deadline (since it's not a real stream, ffmpeg should finish instantly).  But instead got: %v", err)
		}
	}

	pl, err := vs.PollPlaylist(ctx)
	if err != nil {
		t.Errorf("Got error: %v", err)
	}

	if pl.Format != HLS {
		t.Errorf("Expecting HLS Playlist, got %v", pl.Format)
	}

	p, err := m3u8.NewMediaPlaylist(100, 100)
	err = p.DecodeFrom(bytes.NewReader(pl.Data), true)
	if err != nil {
		t.Errorf("Error decoding HLS playlist: %v", err)
	}

	if vs.curSegment != 0 {
		t.Errorf("Segment counter should start with 0.  But got: %v", vs.curSegment)
	}

	seg, err := vs.PollSegment(ctx)

	if vs.curSegment != 1 {
		t.Errorf("Segment counter should move to 1.  But got: %v", vs.curSegment)
	}

	if err != nil {
		t.Errorf("Got error: %v", err)
	}

	if seg.Codec != av.H264 {
		t.Errorf("Expecting H264 segment, got: %v", seg.Codec)
	}

	if seg.Format != HLS {
		t.Errorf("Expecting HLS segment, got %v", seg.Format)
	}

	if seg.Length != time.Second*2 {
		t.Errorf("Expecting 2 sec segments, got %v", seg.Length)
	}

	//Clean up
	os.RemoveAll(workDir)
}

func TestPollPlaylistError(t *testing.T) {
	vs := NewFFMpegVideoSegmenter("./sometestdir", "test", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()
	_, err := vs.PollPlaylist(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expect to exceed deadline, but got: %v", err)
	}
}

func TestPollSegmentError(t *testing.T) {
	vs := NewFFMpegVideoSegmenter("./sometestdir", "test", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()
	_, err := vs.PollSegment(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expect to exceed deadline, but got: %v", err)
	}
}
