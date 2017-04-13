package streaming

import (
	"context"
	"io"
	"testing"

	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/kz26/m3u8"
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

func TestStreamerRegistry(t *testing.T) {
	addr := RandomStreamID()
	streamID := RandomStreamID()
	streamer, _ := NewStreamer(addr)

	firstStream, _ := streamer.AddNewStream()
	_, rs := firstStream.ID.SplitComponents()

	if len(streamer.Streams) != 1 {
		t.Errorf("AddNewStream() didn't add a stream to the streamer")
	}

	resStream, _ := streamer.GetStream(addr, rs)
	if resStream != firstStream {
		t.Errorf("GetStream() didn't return the expected stream")
	}

	// Subscribe to stream
	sid := MakeStreamID(addr, streamID.Str())
	_, err := streamer.SubscribeToStream(sid.String())
	if err != nil {
		t.Errorf("Got an error subscribing to a new stream. %v", err)
	}

	_, err = streamer.SubscribeToStream(sid.String())
	if err == nil {
		t.Errorf("Didn't get an error subscribing to the same stream twice and should have.")
	}
}

type Counter struct {
	Count int8
}
type TestDemux struct {
	c *Counter
}

func (d *TestDemux) Close() error                     { return nil }
func (d *TestDemux) Streams() ([]av.CodecData, error) { return []av.CodecData{}, nil }
func (d *TestDemux) ReadPacket() (av.Packet, error) {
	if d.c.Count == 10 {
		return av.Packet{Idx: d.c.Count}, io.EOF
	}

	d.c.Count = d.c.Count + 1
	return av.Packet{Idx: d.c.Count, Data: []byte{0, 1}}, nil
}

//pubsub.Queue doesn't do anything when calling WriteTrailer, which is an issue here (we need to pass the information along for subscribers).
//Need to fix this, probably extend it.
// func TestSubscribeToRTMP(t *testing.T) {
// 	addr := RandomStreamID()
// 	streamID := RandomStreamID()
// 	streamer, _ := NewStreamer(addr)
// 	id := MakeStreamID(addr, streamID.Str())

// 	// bufLen := len(streamer.rtmpBuffers)
// 	streamsLen := len(streamer.networkStreams)
// 	// if bufLen != 0 {
// 	// 	t.Errorf("Expecting length of 0 for buffer, got %v", bufLen)
// 	// }
// 	if streamsLen != 0 {
// 		t.Errorf("Expecting length of 0 for streams, got %v", streamsLen)
// 	}

// 	q := pubsub.NewQueue()
// 	streamer.SubscribeToRTMPStream(context.Background(), id.String(), "test", q)
// 	// q, err := streamer.SubscribeToRTMPStream(context.Background(), id.String())
// 	// streamer.SubscribeToRTMPStream(context.Background(), id.String())

// 	// bufLen = len(streamer.rtmpBuffers)
// 	streamsLen = len(streamer.networkStreams)
// 	// if bufLen != 1 {
// 	// 	t.Errorf("Expecting length of 1 for buffer, got %v", bufLen)
// 	// }

// 	if streamsLen != 1 {
// 		t.Errorf("Expecting length of 1 for streams, got %v", streamsLen)
// 	}

// 	// if err != nil {
// 	// 	t.Errorf("Got error subscribing to RTMP: %v", err)
// 	// }

// 	strm := streamer.GetNetworkStream(id)
// 	ctx := context.Background()
// 	go strm.WriteRTMPToStream(ctx, &TestDemux{c: &Counter{}})
// 	fmt.Printf("Subscribed to stream: %v\n", id)

// 	c := int8(0)
// 	demux := q.Oldest()
// 	for {
// 		pkt, err := demux.ReadPacket()
// 		fmt.Printf("pkt: %v, err: %v\n", pkt, err)
// 		if err == io.EOF {
// 			break
// 		}
// 		c = c + 1
// 		if c != pkt.Idx {
// 			t.Errorf("Expecting count %v, got %v", c, pkt.Idx)
// 		}
// 	}
// 	if c != 10 {
// 		t.Errorf("Expecting 10 packets, got: %v", c)
// 	}
// }

func TestSubscribeToHLS(t *testing.T) {
	addr := RandomStreamID()
	streamID := RandomStreamID()
	streamer, _ := NewStreamer(addr)
	id := MakeStreamID(addr, streamID.Str())

	// bufLen := len(streamer.hlsBuffers)
	streamsLen := len(streamer.networkStreams)
	// if bufLen != 0 {
	// 	t.Errorf("Expecting length of 0 for buffer, got %v", bufLen)
	// }
	if streamsLen != 0 {
		t.Errorf("Expecting length of 0 for streams, got %v", streamsLen)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := lpmsStream.NewHLSBuffer()
	err := streamer.SubscribeToHLSStream(ctx, id.String(), "local", b)
	if err != nil {
		t.Errorf("Got error when subscribing to hls stream: %v", err)
	}

	// bufLen = len(streamer.hlsBuffers)
	streamsLen = len(streamer.networkStreams)
	// if bufLen != 1 {
	// 	t.Errorf("Expecting length of 1 for buffer, got %v", bufLen)
	// }

	if streamsLen != 1 {
		t.Errorf("Expecting length of 1 for streams, got %v", streamsLen)
	}

	// go func() { //Populate the stream
	strm := streamer.GetNetworkStream(id)
	pl, _ := m3u8.NewMediaPlaylist(15, 15)
	pl.Segments[0] = &m3u8.MediaSegment{URI: "seg1"}
	pl.Segments[1] = &m3u8.MediaSegment{URI: "seg2"}
	strm.WriteHLSPlaylistToStream(*pl)
	strm.WriteHLSSegmentToStream(lpmsStream.HLSSegment{Name: "seg1", Data: []byte("data1")})
	strm.WriteHLSSegmentToStream(lpmsStream.HLSSegment{Name: "seg2", Data: []byte("data2")})
	// 	cancel()
	// }()

	//Wait for the cancel to be finished
	// select {
	// case <-ctx.Done():
	// }

	// ctx := context.Background()
	// fmt.Println("Before Wait and Pop")
	bpl, _ := b.WaitAndPopPlaylist(ctx)
	// fmt.Println("After Wait and Pop")
	bseg1, _ := b.WaitAndPopSegment(ctx, "seg1")
	bseg2, _ := b.WaitAndPopSegment(ctx, "seg2")

	if len(bpl.Segments) != 15 {
		t.Errorf("Playlist length should be 15, but got %v", len(bpl.Segments))
	}

	if bytes.Compare(bseg1, []byte("data1")) != 0 {
		t.Errorf("Segment name shoudl be data1, but got %v", bseg1)
	}

	if bytes.Compare(bseg2, []byte("data2")) != 0 {
		t.Errorf("Segment name shoudl be data2, but got %v", bseg2)
	}
}
