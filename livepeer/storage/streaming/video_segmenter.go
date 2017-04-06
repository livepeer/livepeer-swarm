package streaming

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/kz26/m3u8"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/rtmp"
)

type VideoFormat uint32

var (
	HLS  = MakeVideoFormatType(avFormatTypeMagic + 1)
	RTMP = MakeVideoFormatType(avFormatTypeMagic + 1)
)

func MakeVideoFormatType(base uint32) (c VideoFormat) {
	c = VideoFormat(base) << videoFormatOtherBits
	return
}

const avFormatTypeMagic = 577777
const videoFormatOtherBits = 1

type SegmenterOptions struct {
	EnforceKeyframe bool //Enforce each segment starts with a keyframe
	SegLength       time.Duration
}

type VideoSegment struct {
	Codec  av.CodecType
	Format VideoFormat
	Length time.Duration
	Data   []byte
}

type VideoPlaylist struct {
	Format VideoFormat
	Data   []byte
}

type VideoSegmenter interface{}

//FFMpegVideoSegmenter segments a RTMP stream by invoking FFMpeg and monitoring the file system.
type FFMpegVideoSegmenter struct {
	WorkDir      string
	LocalRtmpUrl string
	StrmID       string
	curSegment   int
	curPlaylist  *m3u8.MediaPlaylist
	curWaitTime  time.Duration
}

func NewFFMpegVideoSegmenter(workDir string, strmID string, localRtmpUrl string) *FFMpegVideoSegmenter {
	return &FFMpegVideoSegmenter{WorkDir: workDir, StrmID: strmID, LocalRtmpUrl: localRtmpUrl}
}

//RTMPToHLS invokes the FFMpeg command to do the segmenting.  This method blocks unless killed.
func (s *FFMpegVideoSegmenter) RTMPToHLS(ctx context.Context, opt SegmenterOptions) error {
	//Set up local workdir
	if _, err := os.Stat(s.WorkDir); os.IsNotExist(err) {
		err := os.Mkdir(s.WorkDir, 0700)
		if err != nil {
			return err
		}
	}

	//Test to make sure local RTMP is running.
	rtmpMux, err := rtmp.Dial(s.LocalRtmpUrl)
	if err != nil {
		glog.Errorf("Video Segmenter Error: %v.  Make sure local RTMP stream is available for segmenter.", err)
		rtmpMux.Close()
		return err
	}
	rtmpMux.Close()

	//Invoke the FFMpeg command
	// fmt.Println("ffmpeg", "-i", fmt.Sprintf("rtmp://localhost:%v/stream/%v", "1935", "test"), "-vcodec", "copy", "-acodec", "copy", "-bsf:v", "h264_mp4toannexb", "-f", "segment", "-muxdelay", "0", "-segment_list", "./tmp/stream.m3u8", "./tmp/stream_%d.ts")
	plfn := fmt.Sprintf("%s/%s.m3u8", s.WorkDir, s.StrmID)
	tsfn := s.WorkDir + "/" + s.StrmID + "_%d.ts"

	//This command needs to be manually killed, because ffmpeg doesn't seem to quit after getting a rtmp EOF
	cmd := exec.Command("ffmpeg", "-i", s.LocalRtmpUrl, "-vcodec", "copy", "-acodec", "copy", "-bsf:v", "h264_mp4toannexb", "-f", "segment", "-muxdelay", "0", "-segment_list", plfn, tsfn)
	err = cmd.Start()
	if err != nil {
		glog.V(logger.Error).Infof("Cannot start ffmpeg command.")
		return err
	}

	ec := make(chan error, 1)
	go func() { ec <- cmd.Wait() }()

	select {
	case ffmpege := <-ec:
		glog.V(logger.Error).Infof("Error from ffmpeg: %v", ffmpege)
		return ffmpege
	case <-ctx.Done():
		//Can't close RTMP server, joy4 doesn't support it.
		//server.Stop()
		cmd.Process.Kill()
		return ctx.Err()
	}
}

//PollSegment monitors the filesystem and returns a new segment as it becomes available
func (s *FFMpegVideoSegmenter) PollSegment(ctx context.Context) (*VideoSegment, error) {
	tsfn := s.WorkDir + "/" + s.StrmID + "_" + strconv.Itoa(s.curSegment) + ".ts"
	seg, err := pollFile(ctx, tsfn, time.Millisecond*100, nil)

	var len time.Duration
	if s.curPlaylist != nil {
		len = time.Second * time.Duration(s.curPlaylist.Segments[s.curSegment].Duration)
	}
	s.curSegment = s.curSegment + 1

	return &VideoSegment{Codec: av.H264, Format: HLS, Length: len, Data: seg}, err
}

//PollPlaylist monitors the filesystem and returns a new playlist as it becomes available
func (s *FFMpegVideoSegmenter) PollPlaylist(ctx context.Context) (*VideoPlaylist, error) {
	plfn := fmt.Sprintf("%s/%s.m3u8", s.WorkDir, s.StrmID)
	var lastPl []byte
	if s.curPlaylist == nil {
		lastPl = nil
	} else {
		lastPl = s.curPlaylist.Encode().Bytes()
	}

	pl, err := pollFile(ctx, plfn, time.Second*1, lastPl)
	if err != nil {
		return nil, err
	}

	p, err := m3u8.NewMediaPlaylist(100, 100)
	err = p.DecodeFrom(bytes.NewReader(pl), true)
	if err != nil {
		return nil, err
	}

	s.curPlaylist = p
	return &VideoPlaylist{Format: HLS, Data: p.Encode().Bytes()}, err
}

func pollFile(ctx context.Context, fn string, sleepTime time.Duration, lastFile []byte) (f []byte, err error) {
	for {
		if _, err := os.Stat(fn); err == nil {
			content, err := ioutil.ReadFile(fn)
			if err != nil {
				return nil, err
			}

			if lastFile == nil || bytes.Compare(lastFile, content) != 0 {
				return content, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		time.Sleep(sleepTime)
	}
}
