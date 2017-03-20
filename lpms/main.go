package main

import "context"
import "fmt"
import joy4rtmp "github.com/nareix/joy4/format/rtmp"
import "github.com/livepeer/go-livepeer/lpms/io"

func main() {
	fmt.Println("hello")
	server := &joy4rtmp.Server{Addr: ":1936"}
	var in *io.RTMPStream

	server.HandlePlay = func(conn *joy4rtmp.Conn) {
		dst := &io.RTMPStream{Output: conn}
		dst.Stream(context.Background(), in)
		// fmt.Println("")
	}

	server.HandlePublish = func(conn *joy4rtmp.Conn) {
		in = &io.RTMPStream{Input: conn}
	}

	server.ListenAndServe()
}