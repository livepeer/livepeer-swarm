package io

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/nareix/joy4/av"
)

var ErrBufferFull = errors.New("Stream Buffer Full")
var ErrBufferEmpty = errors.New("Stream Buffer Empty")
var ErrBufferItemType = errors.New("Buffer Item Type Not Recognized")

type BufferItemType uint8

const (
	RTMPHeader = BufferItemType(iota + 1) // 8-bit unsigned integer
	RTMPPacket
	RTMPEOF
	HLSPlaylist
	HLSSegment
)

type LPMSBuffer interface {
	Push(in []byte) error
	Pop() ([]byte, error)
}

type StreamBuffer struct {
	Len int32
}

type BufferItem struct {
	Type BufferItemType
	Data interface{}
}

func (b *StreamBuffer) Push(in []byte) error {
	// fmt.Println("push", in)
	b.Len = b.Len + 1
	return nil
}

func (b *StreamBuffer) Pop() ([]byte, error) {
	if b.Len == 0 {
		return nil, ErrBufferEmpty
	}
	b.Len = b.Len - 1
	return nil, nil
}

type Stream struct {
	Buffer   LPMSBuffer
	StreamID string
	// StreamFormat io.Format
}

//ReadRTMPFromStream reads the content from the RTMP stream out into the dst.
func (s *Stream) ReadRTMPFromStream(ctx context.Context, dst av.MuxCloser) error {
	defer dst.Close()

	//TODO: Put this for loop in a go routine and handle errors through a channel
	for {
		bytes, err := s.Buffer.Pop()
		if err != nil {
			return err
		}

		bufferItem, err := Deserialize(bytes)
		// fmt.Println("item: ", bufferItem)
		switch bufferItem.Type {
		case RTMPHeader:

			headers := bufferItem.Data.([]av.CodecData)
			err = dst.WriteHeader(headers)
			if err != nil {
				glog.V(logger.Error).Infof("Error writing RTMP header from Stream %v to mux", s.StreamID)
				return err
			}
		case RTMPPacket:
			packet := bufferItem.Data.(av.Packet)
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
			glog.V(logger.Error).Infof("Cannot recognize buffer iteam type: ", bufferItem.Type)
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
			headerBytes, err := Serialize(&BufferItem{Type: RTMPHeader, Data: header})
			if err != nil {
				return err
			}
			err = s.Buffer.Push(headerBytes)
			// fmt.Println("pushing header", headerBytes)
			if err != nil {
				return err
			}

			// var lastKeyframe av.Packet
			for {
				packet, err := src.ReadPacket()
				if err == io.EOF {
					//TODO: Push EOF Packet
					return err
				} else if err != nil {
					return err
				}

				if packet.IsKeyFrame {
					// lastKeyframe = packet
				}

				packetBytes, err := Serialize(&BufferItem{Type: RTMPPacket, Data: packet})
				if err != nil {
					return err
				}
				err = s.Buffer.Push(packetBytes)
				// fmt.Println("pushing packet", packetBytes)
				if err == ErrBufferFull {
					//Delete all packets until last keyframe, insert headers in front - trying to get rid of streaming artifacts.
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
func Serialize(bi *BufferItem) ([]byte, error) {
	// gob.Register(map[string]interface{}{})
	gob.Register([]av.CodecData{})
	gob.Register(av.Packet{})
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(bi)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

//Deserialize converts []byte into BufferItem
func Deserialize(in []byte) (*BufferItem, error) {
	bi := &BufferItem{}
	b := bytes.Buffer{}
	b.Write(in)
	d := gob.NewDecoder(&b)
	err := d.Decode(bi)
	if err != nil {
		return bi, err
	}
	return bi, nil
}

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
