package io

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/nareix/joy4/av"
)

//Testing WriteRTMP errors
var ErrPacketRead = errors.New("packet read error")
var ErrStreams = errors.New("streams error")

type BadStreamsDemuxer struct{}

func (d BadStreamsDemuxer) Close() error                     { return nil }
func (d BadStreamsDemuxer) Streams() ([]av.CodecData, error) { return nil, ErrStreams }
func (d BadStreamsDemuxer) ReadPacket() (av.Packet, error)   { return av.Packet{}, nil }

type BadPacketsDemuxer struct{}

func (d BadPacketsDemuxer) Close() error                     { return nil }
func (d BadPacketsDemuxer) Streams() ([]av.CodecData, error) { return nil, nil }
func (d BadPacketsDemuxer) ReadPacket() (av.Packet, error)   { return av.Packet{}, ErrPacketRead }

func TestWriteRTMPErrors(t *testing.T) {
	stream := Stream{Buffer: &StreamBuffer{}, StreamID: "test"}
	err := stream.WriteRTMPToStream(context.Background(), BadStreamsDemuxer{})
	if err != ErrStreams {
		t.Error("Expecting Streams Error, but got: ", err)
	}

	err = stream.WriteRTMPToStream(context.Background(), BadPacketsDemuxer{})
	if err != ErrPacketRead {
		t.Error("Expecting Packet Read Error, but got: ", err)
	}
}

//Testing WriteRTMP
type PacketsDemuxer struct {
	c *Counter
}

type Counter struct {
	Count int
}

func (d PacketsDemuxer) Close() error                     { return nil }
func (d PacketsDemuxer) Streams() ([]av.CodecData, error) { return nil, nil }
func (d PacketsDemuxer) ReadPacket() (av.Packet, error) {
	if d.c.Count == 10 {
		return av.Packet{}, io.EOF
	} else {
		d.c.Count = d.c.Count + 1
		return av.Packet{}, nil
	}
}

func TestWriteRTMP(t *testing.T) {
	stream := Stream{Buffer: &StreamBuffer{Len: 0}, StreamID: "test"}
	err := stream.WriteRTMPToStream(context.Background(), PacketsDemuxer{c: &Counter{Count: 0}})

	if err != io.EOF {
		t.Error("Expecting EOF, but got: ", err)
	}

	if stream.Buffer.(*StreamBuffer).Len != 11 { //10 packets, 1 header
		t.Error("Expecting buffer length to be 11, but got: ", stream.Buffer.(*StreamBuffer).Len)
	}

}

//Test ReadRTMP Errors
type FakeStreamBuffer struct {
	c *Counter
}

func (b *FakeStreamBuffer) Push(in []byte) error { return nil }
func (b *FakeStreamBuffer) Pop() ([]byte, error) {
	// fmt.Println("pop, count:", b.c.Count)
	switch b.c.Count {
	case 10:
		b.c.Count = b.c.Count - 1
		i := &BufferItem{Type: RTMPHeader, Data: []av.CodecData{}}
		h, _ := Serialize(i)
		return h, nil
	case 0:
		return nil, ErrBufferEmpty
	default:
		b.c.Count = b.c.Count - 1
		i := &BufferItem{Type: RTMPPacket, Data: av.Packet{}}
		// fmt.Println("item before: ", i)
		p, _ := Serialize(i)
		// i, err := Deserialize(p)
		// if err != nil {
		// 	fmt.Println(err)
		// }
		// fmt.Println("item after: ", i)
		return p, nil
	}
}

var ErrBadHeader = errors.New("BadHeader")
var ErrBadPacket = errors.New("BadPacket")

type BadHeaderMuxer struct{}

func (d BadHeaderMuxer) Close() error                     { return nil }
func (d BadHeaderMuxer) WriteHeader([]av.CodecData) error { return ErrBadHeader }
func (d BadHeaderMuxer) WriteTrailer() error              { return nil }
func (d BadHeaderMuxer) WritePacket(av.Packet) error      { return nil }

type BadPacketMuxer struct{}

func (d BadPacketMuxer) Close() error                     { return nil }
func (d BadPacketMuxer) WriteHeader([]av.CodecData) error { return nil }
func (d BadPacketMuxer) WriteTrailer() error              { return nil }
func (d BadPacketMuxer) WritePacket(av.Packet) error      { return ErrBadPacket }

func TestReadRTMPError(t *testing.T) {
	stream := Stream{Buffer: &FakeStreamBuffer{c: &Counter{Count: 10}}, StreamID: "test"}
	err := stream.ReadRTMPFromStream(context.Background(), BadHeaderMuxer{})

	if err != ErrBadHeader {
		t.Error("Expecting bad header error, but got ", err)
	}

	err = stream.ReadRTMPFromStream(context.Background(), BadPacketMuxer{})
	if err != ErrBadPacket {
		t.Error("Expecting bad packet error, but got ", err)
	}
}

//Test ReadRTMP

type PacketsMuxer struct{}

func (d PacketsMuxer) Close() error                     { return nil }
func (d PacketsMuxer) WriteHeader([]av.CodecData) error { return nil }
func (d PacketsMuxer) WriteTrailer() error              { return nil }
func (d PacketsMuxer) WritePacket(av.Packet) error      { return nil }

func TestReadRTMP(t *testing.T) {
	fakeBuffer := &FakeStreamBuffer{c: &Counter{Count: 10}}
	stream := Stream{Buffer: fakeBuffer, StreamID: "test"}
	err := stream.ReadRTMPFromStream(context.Background(), PacketsMuxer{})

	if err != ErrBufferEmpty {
		t.Error("Expecting buffer to be empty, but got ", err)
	}

	if fakeBuffer.c.Count != 0 {
		t.Error("Expecting buffer length to be 0, but got ", fakeBuffer.c.Count)
	}
}
