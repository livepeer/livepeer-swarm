package main

import "context"
import "fmt"
import "github.com/livepeer/go-livepeer/lpms/vidplayer"
import "github.com/livepeer/go-livepeer/lpms/vidlistener"
import "github.com/livepeer/go-livepeer/lpms/io"
import joy4rtmp "github.com/nareix/joy4/format/rtmp"
import "github.com/nareix/joy4/av"

type StreamDB struct {
	db map[string]*io.Stream
}

func main() {
	// lpms.NewServer()
	server := &joy4rtmp.Server{Addr: ":1936"}
	player := &vidplayer.VidPlayer{RtmpServer: server}
	listener := &vidlistener.VidListener{RtmpServer: server, Streams: make(map[string]vidlistener.LocalStream)}	
	streamDB := &StreamDB{db: make(map[string]*io.Stream)}

	player.HandleRTMPPlay(func(ctx context.Context, reqPath string, dst av.MuxCloser) error {
		src := streamDB.db[reqPath]

		if src != nil {
			src.ReadRTMPFromStream(ctx, dst)
		} else {
			fmt.Println("Cannot find stream for ", reqPath)
		}
		return nil
	})

	listener.HandleRTMPPublish(
		func(reqPath string, streamID chan<- string) error {
			streamID <- reqPath
			return nil
		},
		func(ctx context.Context, reqPath string, src av.DemuxCloser) error {
			stream := io.NewStream(reqPath)
			streamDB.db[reqPath] = stream

			c := make(chan error, 0)
			go func() {c <- stream.WriteRTMPToStream(ctx, src)}()
			select {
				case err := <- c:
				fmt.Println("Got error writing: ", err)
			}
			return nil
		})

	server.ListenAndServe()
}