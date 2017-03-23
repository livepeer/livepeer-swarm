package io

import (
	"context"
	"errors"
	"io"
	"reflect"

	"time"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/kz26/m3u8"
	"github.com/nareix/joy4/av"
)

var ErrBufferFull = errors.New("Stream Buffer Full")
var ErrBufferEmpty = errors.New("Stream Buffer Empty")
var ErrBufferItemType = errors.New("Buffer Item Type Not Recognized")
var ErrDroppedRTMPStream = errors.New("RTMP Stream Stopped Without EOF")
var ErrHttpReqFailed = errors.New("Http Request Failed")

type RTMPEOF struct{}

type streamBuffer struct {
	q *Queue
}

func newStreamBuffer() *streamBuffer {
	return &streamBuffer{q: NewQueue(1000)}
}

func (b *streamBuffer) push(in interface{}) error {
	b.q.Put(in)
	return nil
}

func (b *streamBuffer) poll(wait time.Duration) (interface{}, error) {
	results, err := b.q.Poll(1, wait)
	if err != nil {
		return nil, err
	}
	result := results[0]
	return result, nil
}

func (b *streamBuffer) pop() (interface{}, error) {
	results, err := b.q.Get(1)
	if err != nil {
		return nil, err
	}
	result := results[0]
	return result, nil
}

func (b *streamBuffer) len() int64 {
	return b.q.Len()
}

type HLSSegment struct {
	Name string
	Data []byte
}

type Stream struct {
	StreamID    string
	RTMPTimeout time.Duration
	HLSTimeout  time.Duration
	buffer      *streamBuffer
}

func (s *Stream) Len() int64 {
	return s.buffer.len()
}

func NewStream(id string) *Stream {
	return &Stream{buffer: newStreamBuffer(), StreamID: id}
}

//ReadRTMPFromStream reads the content from the RTMP stream out into the dst.
func (s *Stream) ReadRTMPFromStream(ctx context.Context, dst av.MuxCloser) error {
	defer dst.Close()

	//TODO: Make sure to listen to ctx.Done()
	for {
		item, err := s.buffer.poll(s.RTMPTimeout)
		if err != nil {
			return err
		}

		switch item.(type) {
		case []av.CodecData:
			headers := item.([]av.CodecData)
			err = dst.WriteHeader(headers)
			if err != nil {
				glog.V(logger.Error).Infof("Error writing RTMP header from Stream %v to mux", s.StreamID)
				return err
			}
		case av.Packet:
			packet := item.(av.Packet)
			err = dst.WritePacket(packet)
			if err != nil {
				glog.V(logger.Error).Infof("Error writing RTMP packet from Stream %v to mux", s.StreamID)
				return err
			}
		case RTMPEOF:
			err := dst.WriteTrailer()
			if err != nil {
				glog.V(logger.Error).Infof("Error writing RTMP trailer from Stream %v", s.StreamID)
				return err
			}
			return io.EOF
		default:
			glog.V(logger.Error).Infof("Cannot recognize buffer iteam type: ", reflect.TypeOf(item))
			return ErrBufferItemType
		}
	}
}

//WriteRTMPToStream writes a video stream from src into the stream.
func (s *Stream) WriteRTMPToStream(ctx context.Context, src av.DemuxCloser) error {
	defer src.Close()

	c := make(chan error, 1)
	go func() {
		c <- func() error {
			header, err := src.Streams()
			if err != nil {
				return err
			}
			err = s.buffer.push(header)
			if err != nil {
				return err
			}

			// var lastKeyframe av.Packet
			for {
				packet, err := src.ReadPacket()
				if err == io.EOF {
					s.buffer.push(RTMPEOF{})
					return err
				} else if err != nil {
					return err
				} else if len(packet.Data) == 0 { //TODO: Investigate if it's possible for packet to be nil (what happens when RTMP stopped publishing because of a dropped connection? Is it possible to have err and packet both nil?)
					return ErrDroppedRTMPStream
				}

				if packet.IsKeyFrame {
					// lastKeyframe = packet
				}

				err = s.buffer.push(packet)
				if err == ErrBufferFull {
					//TODO: Delete all packets until last keyframe, insert headers in front - trying to get rid of streaming artifacts.
				}
			}
		}()
	}()

	select {
	case <-ctx.Done():
		glog.V(logger.Error).Infof("Finished writing RTMP to Stream %v", s.StreamID)
		return ctx.Err()
	case err := <-c:
		return err
	}
}

func (s *Stream) WriteHLSPlaylistToStream(pl m3u8.MediaPlaylist) error {
	return s.buffer.push(pl)
}

func (s *Stream) WriteHLSSegmentToStream(seg HLSSegment) error {
	return s.buffer.push(seg)
}

//ReadHLSFromStream reads an HLS stream into an HLSBuffer
func (s *Stream) ReadHLSFromStream(buffer HLSMuxer) error {
	for {
		item, err := s.buffer.poll(s.HLSTimeout)
		if err != nil {
			return err
		}

		switch item.(type) {
		case m3u8.MediaPlaylist:
			buffer.WritePlaylist(item.(m3u8.MediaPlaylist))
		case HLSSegment:
			buffer.WriteSegment(item.(HLSSegment).Name, item.(HLSSegment).Data)
		default:
			return ErrBufferItemType
		}
	}
	return nil
}

//WriteHLSToStream puts a video stream from a HLS demuxer into a stream.  This method does NOT check the validity of the
//video stream itself.  So there is NO guarentee that the playlist will match the segments.  It simply puts everything into a buffer queue.
//THIS MIGHT NOT BE USEFUL!!!
// func (s *Stream) WriteHLSToStream(ctx context.Context, hlsDemux HLSDemuxer) error { // playlistUrl string) error {
// 	pec := make(chan error)
// 	go func() {
// 		pec <- func() error {
// 			for {
// 				pl, err := hlsDemux.WaitAndPopPlaylist(ctx)
// 				if err != nil {
// 					return err
// 				}
// 				s.WriteHLSPlaylistToStream(pl)
// 			}
// 		}()
// 	}()

// 	sec := make(chan error)
// 	go func() {
// 		sec <- func() error {
// 			for {
// 				pl, err := hlsDemux.WaitAndPopSegment(ctx, "")
// 				if err != nil {
// 					return err
// 				}
// 				s.WriteHLSPlaylistToStream(pl)
// 			}
// 		}()
// 	}()

// 	return nil
// pe := make(chan error, 1)
// se := make(chan error, 1)
// playlistChan := make(chan m3u8.MediaPlaylist, 1)
// segmentChan := make(chan []byte)
// // context.WithValue
// // cancel := context.CancelFunc(ctx)

// go func() { pe <- hlsDemux.PollPlaylist(ctx, playlistChan) }()
// go func() { se <- hlsDemux.PollSegment(ctx, segmentChan) }()

// for {
// 	select {
// 	case perr := <-pe:
// 		glog.V(logger.Error).Infof("Stream HLS Write Error On Playlist: %v, %v", s.StreamID, perr)
// 		return perr
// 	case serr := <-se:
// 		glog.V(logger.Error).Infof("Stream HLS Write Error on Segment: %v, %v", s.StreamID, serr)
// 		return serr
// 	case playlist := <-playlistChan:
// 		s.buffer.push(playlist)
// 	case segment := <-segmentChan:
// 		s.buffer.push(segment)
// 	case <-ctx.Done():
// 		glog.V(logger.Error).Infof("Stream HLS Write Killed: %v, %v", s.StreamID, ctx.Err())
// 		return ctx.Err()
// 	}
// }
// }

//Serialize converts BufferItem into []byte, so it can be put on the wire.
// func Serialize(bi *BufferItem) ([]byte, error) {
// 	// gob.Register(map[string]interface{}{})
// 	gob.Register([]av.CodecData{})
// 	gob.Register(av.Packet{})
// 	b := bytes.Buffer{}
// 	e := gob.NewEncoder(&b)
// 	err := e.Encode(bi)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return b.Bytes(), nil
// }

// //Deserialize converts []byte into BufferItem
// func Deserialize(in []byte) (*BufferItem, error) {
// 	bi := &BufferItem{}
// 	b := bytes.Buffer{}
// 	b.Write(in)
// 	d := gob.NewDecoder(&b)
// 	err := d.Decode(bi)
// 	if err != nil {
// 		return bi, err
// 	}
// 	return bi, nil
// }

// type Stream interface {
// 	Muxer
// 	Demuxer
// 	// Write(ctx context.Context, input Stream) error
// 	// Read(ctx context.Context, req StreamRequest) error
// 	Close() error
// 	// Start(chan Packet)
// }

// type Muxer interface {
// 	WritePacket(p Packet) error
// }

// type Demuxer interface {
// 	ReadPacket(ctx context.Context) (Packet, error)
// }

// type RTMPMuxer interface {
// 	av.Muxer
// }

// type RTMPDemuxer interface {
// 	av.PacketReader
// }

// type RTMPStream struct {
// 	RTMPMuxer
// 	RTMPDemuxer

// 	streamHeaders []av.CodecData
// 	streamBuffer  []RTMPPacket
// }

// type RTMPPacket struct {
// 	Packet av.Packet
// }

// func (self *RTMPStream) Write(p RTMPPacket) error {
// 	//Save to local buffer
// 	return nil
// 	// self.mux.WritePacket(p.Packet)
// }

// func Read(ctx context.Context) (Packet, error) {
// 	//returns from local buffer
// 	return nil, nil
// }

// func (self *RTMPStream) Stream(ctx context.Context, mux RTMPMuxer) (Packet, error) {
// 	c := make(chan error, 1)
// 	go func() { c <- self.stream(mux) }()
// 	select {
// 	case <-ctx.Done():
// 		self.Close()
// 		<-c
// 		return ctx.Err()
// 	case err := <-c:
// 		return err
// 	}
// }

// // func (self *RTMPStream) Stream(ctx context.Context, input *RTMPStream) error {
// // 	c := make(chan error, 1)
// // 	go func() { c <- self.stream(input) }()
// // 	select {
// // 	case <-ctx.Done():
// // 		self.Close()
// // 		<-c
// // 		return ctx.Err()
// // 	case err := <-c:
// // 		return err
// // 	}
// // }

// func (self *RTMPStream) Close() error {
// 	//Close the stream
// 	return nil
// }

// func (self *RTMPStream) stream(mux RTMPMuxer) error {
// 	//return error if dst doesn't respond for a while.
// 	// in.RtmpInput.
// 	err := mux.mux.WriteHeader(self.streamHeaders)
// 	// headers, err := in.Input.Streams()
// 	if err != nil {
// 		glog.V(logger.Info).Infof("Error reading RTMP stream header: %v", err)
// 	}
// 	// self.Output.WriteHeader(headers)

// 	for {
// 		packet, err := self.streamBuffer.ReadPacket()
// 		if err == io.EOF {
// 			glog.V(logger.Info).Infof("Writing RTMP EOF")
// 			err := mux.mux.WriteTrailer()
// 			if err != nil {
// 				glog.V(logger.Info).Infof("Error writing RTMP trailer: %v", err)
// 				return err
// 			}
// 		}
// 		err = mux.mux.WritePacket(packet)
// 		if packet.Idx%100 == 0 {
// 			glog.V(logger.Info).Infof("Copy RTMP to muxer from channel. %d", packet.Idx)
// 		}

// 		if err != nil {
// 			glog.V(logger.Error).Infof("Error writing packet to video player: %v", err)
// 			return err
// 		}
// 	}

// 	return nil
// }
