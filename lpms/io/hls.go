package io

import (
	"context"
	"errors"
	"fmt"
	"time"

	cmap "github.com/orcaman/concurrent-map"

	"sync"

	"github.com/kz26/m3u8"
)

var ErrNotFound = errors.New("Not Found")

type HLSDemuxer interface {
	//This method should ONLY push a playlist onto a chan when it's a NEW playlist
	WaitAndPopPlaylist(ctx context.Context) (m3u8.MediaPlaylist, error)
	//This method should ONLY push a segment onto a chan when it's a NEW segment
	WaitAndPopSegment(ctx context.Context, name string) ([]byte, error)

	// ReadPlaylist() (m3u8.MediaPlaylist, error)
	// ReadSegment(name string) (m3u8.MediaSegment, error)
}

type HLSMuxer interface {
	WritePlaylist(m3u8.MediaPlaylist) error
	WriteSegment(name string, s []byte) error
}

//TODO: Write tests, set buffer size, kick out segments / playlists if too full
type HLSBuffer struct {
	HoldTime   time.Duration
	plCacheNew bool
	segCache   *Queue
	// pq       *Queue
	plCache m3u8.MediaPlaylist
	sq      *cmap.ConcurrentMap
	lock    sync.Locker
}

func NewHLSBuffer() *HLSBuffer {
	m := cmap.New()
	return &HLSBuffer{plCacheNew: false, segCache: &Queue{}, HoldTime: time.Second, sq: &m, lock: &sync.Mutex{}}
}

func (b *HLSBuffer) WritePlaylist(p m3u8.MediaPlaylist) error {
	// err := b.pq.Put(p)
	// if err != nil {
	// 	return err
	// }
	// b.pubPl <- true
	b.lock.Lock()
	b.plCache = p
	b.plCacheNew = true
	b.lock.Unlock()
	return nil
}

// func (b *HLSBuffer) WriteSegment(s m3u8.MediaSegment) error {
func (b *HLSBuffer) WriteSegment(name string, s []byte) error {
	b.lock.Lock()
	b.segCache.Put(name)
	b.sq.Set(name, s)
	b.lock.Unlock()
	return nil
}

//Returns the playlist synchronously.
func (b *HLSBuffer) WaitAndPopPlaylist(ctx context.Context) (m3u8.MediaPlaylist, error) {
	for {
		// b.lock.Lock()
		//TODO: This is a problem here - potential race condition.
		if b.plCacheNew {
			// pc <- b.plCache
			return b.plCache, nil
			b.plCacheNew = false
		}
		// b.lock.Unlock()
		time.Sleep(time.Second * 1)
		select {
		case <-ctx.Done():
			return m3u8.MediaPlaylist{}, ctx.Err()
		default:
			//Fall through here so we can loop back
		}
	}
}

func (b *HLSBuffer) WaitAndPopSegment(ctx context.Context, name string) ([]byte, error) {
	for {
		// b.lock.Lock()
		//We have a race condition here
		// if b.segCache.Len() > 0 {
		// 	sn, err := b.segCache.Get(1)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	seg, found := b.sq.Get(sn[0].(string))
		// 	if found {
		// 		sc <- seg.([]byte)
		// 	}
		// }
		seg, found := b.sq.Get(name)
		// fmt.Println(b.sq)
		fmt.Printf("GetSegment: %v, %v", name, found)
		// fmt.Println(b.sq.Items())
		if found {
			b.sq.Remove(name)
			return seg.([]byte), nil
		}

		time.Sleep(time.Second * 1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			//Fall through here so we can loop back
		}
	}
}

// func (b *HLSBuffer) ReadPlaylist() (m3u8.Playlist, error) {
// 	ls, err := b.pq.Get(1)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return ls[0].(m3u8.Playlist), nil
// }

// func (b *HLSBuffer) ReadSegment(name string) (m3u8.MediaSegment, error) {
// 	s, suc := b.sq.Get(name)
// 	if suc == false {
// 		return m3u8.MediaSegment{}, ErrNotFound
// 	}
// 	return s.(m3u8.MediaSegment), nil
// }
