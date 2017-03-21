package io

import (
	"context"
	"errors"
	"io"
	"reflect"

	"time"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/nareix/joy4/av"
)

var ErrBufferFull = errors.New("Stream Buffer Full")
var ErrBufferEmpty = errors.New("Stream Buffer Empty")
var ErrBufferItemType = errors.New("Buffer Item Type Not Recognized")
var ErrDroppedRTMPStream = errors.New("RTMP Stream Stopped Without EOF")

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

type Stream struct {
	StreamID    string
	RTMPTimeout time.Duration
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
