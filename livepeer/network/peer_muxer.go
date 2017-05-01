package network

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/kz26/m3u8"
	"github.com/livepeer/go-livepeer/livepeer/streaming"
	lpmsStream "github.com/livepeer/lpms/stream"
	"github.com/nareix/joy4/av"
)

type peerMuxer struct {
	originNode common.Hash
	streamID   string
	peer       *peer
}

func (p *peerMuxer) WritePlaylist(pl m3u8.MediaPlaylist) error {
	// glog.Infof("Writing pl to peer", p.peer.Addr())
	chunk := streaming.VideoChunk{
		ID:   streaming.DeliverStreamMsgID,
		M3U8: pl.Encode().Bytes(),
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WriteSegment(name string, s []byte) error {
	// glog.Infof("Writing segment to peer %v", p.peer.Addr())
	chunk := streaming.VideoChunk{
		ID:         streaming.DeliverStreamMsgID,
		HLSSegData: s,
		HLSSegName: name,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WriteHeader(header []av.CodecData) error {
	chunk := streaming.VideoChunk{
		ID:            streaming.DeliverStreamMsgID,
		Seq:           0,
		HeaderStreams: header,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WritePacket(pkt av.Packet) error {
	chunk := streaming.VideoChunk{
		ID:     streaming.DeliverStreamMsgID,
		Packet: pkt,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WriteTrailer() error {
	// glog.Infof("Writing trailer to peer")
	chunk := streaming.VideoChunk{
		ID: streaming.EOFStreamMsgID,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		Format:     lpmsStream.RTMP,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.EOFStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}
