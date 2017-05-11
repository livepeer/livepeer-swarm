package streaming

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	lpmsStream "github.com/livepeer/lpms/stream"
	"github.com/nareix/joy4/av"
)

func TestStreamID(t *testing.T) {
	nodeID := common.HexToHash("0x3ee489d01ab49caf1be0c824f2c913705e97d359ebbdd19a7389700cb8b7114d")
	streamID := "c8fdf676c39cb0a01133562c4fd81743e012a6107dc544e3555a24296aeaed23"

	res := MakeStreamID(nodeID, streamID)
	expected := "3ee489d01ab49caf1be0c824f2c913705e97d359ebbdd19a7389700cb8b7114dc8fdf676c39cb0a01133562c4fd81743e012a6107dc544e3555a24296aeaed23"

	if res.String() != expected {
		t.Errorf("MakeStreamID returned %v and should have returned %v", res, expected)
	}

	rn, rs := res.SplitComponents()
	if rn != nodeID || rs != streamID {
		t.Errorf("SplitComponents returned %v, %v", rn, rs)
	}
}

func TestNetworkStream(t *testing.T) {
	addr := RandomStreamID()
	// streamID := RandomStreamID()
	streamer, _ := NewStreamer(addr)
	strm, _ := streamer.AddNewNetworkStream(lpmsStream.HLS)
	strmLen := len(streamer.networkStreams)
	if strmLen != 1 {
		t.Errorf("Expecting 1 stream, got %v", strmLen)
	}

	streamer.DeleteNetworkStream(StreamID(strm.GetStreamID()))
	strmLen = len(streamer.networkStreams)
	if strmLen != 0 {
		t.Errorf("Expecting 0 stream, got %v", strmLen)
	}
}

type Counter struct {
	Count int8
}

type TestQueue struct {
	c            *Counter
	wroteTrailer bool
}

func (d *TestQueue) Close() error                     { return nil }
func (d *TestQueue) Streams() ([]av.CodecData, error) { return []av.CodecData{}, nil }
func (d *TestQueue) ReadPacket() (av.Packet, error) {
	if d.c.Count == 0 {
		return av.Packet{Idx: d.c.Count}, io.EOF
	}

	d.c.Count = d.c.Count - 1
	return av.Packet{Idx: d.c.Count, Data: []byte{0, 1}}, nil
}

func (d *TestQueue) WriteHeader([]av.CodecData) error {
	return nil
}

func (d *TestQueue) WriteTrailer() error {
	d.wroteTrailer = true
	return nil
}

func (d *TestQueue) WritePacket(av.Packet) error {
	// fmt.Println("Writing packet")
	d.c.Count = d.c.Count + 1
	return nil
}

func TestSubscribeToRTMP(t *testing.T) {
	addr := RandomStreamID()
	streamID := RandomStreamID()
	streamer, _ := NewStreamer(addr)
	id := MakeStreamID(addr, streamID.Str())

	streamsLen := len(streamer.networkStreams)

	if streamsLen != 0 {
		t.Errorf("Expecting length of 0 for streams, got %v", streamsLen)
	}

	q := &TestQueue{c: &Counter{Count: 0}}
	ctx := context.Background()
	streamer.SubscribeToRTMPStream(id.String(), "test", q)

	streamsLen = len(streamer.networkStreams)
	if streamsLen != 1 {
		t.Errorf("Expecting length of 1 for streams, got %v", streamsLen)
	}

	strm := streamer.GetNetworkStream(id)
	ec := make(chan error)
	go func() { ec <- strm.WriteRTMPToStream(ctx, &TestQueue{c: &Counter{Count: 10}}) }()

	select {
	case err := <-ec:
		fmt.Printf("Got err: %v\n", err)
	}

	time.Sleep(1 * time.Second) //This is a terrible hack... But we have no way of blocking for SubscribeToRTMPStream until a EOF.  Need to be fixed

	c := int8(0)
	for {
		pkt, err := q.ReadPacket()
		// fmt.Printf("pkt: %v, err: %v\n", pkt, err)
		if err == io.EOF {
			break
		}
		c = c + 1
		if c != (10 - pkt.Idx) {
			t.Errorf("Expecting count %v, got %v", c, pkt.Idx)
		}
	}
	if c != 10 {
		t.Errorf("Expecting 10 packets, got: %v", c)
	}
}

func TestSubscribeToHLS(t *testing.T) {
	addr := RandomStreamID()
	streamID := RandomStreamID()
	streamer, _ := NewStreamer(addr)
	id := MakeStreamID(addr, streamID.Str())

	streamsLen := len(streamer.networkStreams)

	if streamsLen != 0 {
		t.Errorf("Expecting length of 0 for streams, got %v", streamsLen)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	b := lpmsStream.NewHLSBuffer(10, 100)
	err := streamer.SubscribeToHLSStream(id.String(), "local", b)
	if err != nil {
		t.Errorf("Got error when subscribing to hls stream: %v", err)
	}

	streamsLen = len(streamer.networkStreams)

	if streamsLen != 1 {
		t.Errorf("Expecting length of 1 for streams, got %v", streamsLen)
	}

	strm := streamer.GetNetworkStream(id)
	// pl, _ := m3u8.NewMediaPlaylist(15, 15)
	// pl.Segments[0] = &m3u8.MediaSegment{URI: "seg_1.ts"}
	// pl.Segments[1] = &m3u8.MediaSegment{URI: "seg_2.ts"}
	// strm.WriteHLSPlaylistToStream(*pl)
	strm.WriteHLSSegmentToStream(lpmsStream.HLSSegment{SeqNo: 1, Name: "seg_1.ts", Data: []byte("data1")})
	strm.WriteHLSSegmentToStream(lpmsStream.HLSSegment{SeqNo: 2, Name: "seg_2.ts", Data: []byte("data2")})

	time.Sleep(time.Millisecond * 200) //Sleep to wait for the segment to propagate. This is a total hack.

	// bpl, _ := b.WaitAndPopPlaylist(ctx)
	// bpl, _ := b.GeneratePlaylist()
	bpl, _ := b.LatestPlaylist()
	bseg1, _ := b.WaitAndPopSegment(ctx, "seg_1.ts")
	bseg2, _ := b.WaitAndPopSegment(ctx, "seg_2.ts")

	if bpl.Count() != 2 {
		t.Errorf("Playlist length should be 2, but got %v", bpl.Count())
	}

	if bytes.Compare(bseg1, []byte("data1")) != 0 {
		t.Errorf("Segment name shoudl be data1, but got %v", bseg1)
	}

	if bytes.Compare(bseg2, []byte("data2")) != 0 {
		t.Errorf("Segment name shoudl be data2, but got %v", bseg2)
	}
}

func TestUnsubscribeToHLS(t *testing.T) {
	addr := RandomStreamID()
	streamID := RandomStreamID()
	streamer, _ := NewStreamer(addr)
	id := MakeStreamID(addr, streamID.Str())

	streamsLen := len(streamer.networkStreams)
	if streamsLen != 0 {
		t.Errorf("Expecting length of 0 for streams, got %v", streamsLen)
	}

	b := lpmsStream.NewHLSBuffer(10, 100)
	err := streamer.SubscribeToHLSStream(id.String(), "local", b)

	if err != nil {
		t.Errorf("Got error %v subscribing to stream", err)
	}

	streamsLen = len(streamer.networkStreams)
	if streamsLen != 1 {
		t.Errorf("Expecting 1 stream, got %v", streamsLen)
	}

	streamer.UnsubscribeToHLSStream(id.String(), "local")

	streamsLen = len(streamer.networkStreams)
	if streamsLen != 0 {
		t.Errorf("Expecting 0 stream, got %v", streamsLen)
	}

	subLen := len(streamer.subscribers)
	if subLen != 0 {
		t.Errorf("Expecting 0 subscribers, got %v", subLen)
	}
}
