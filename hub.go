package chitosocket

import (
	"slices"

	"github.com/puzpuzpuz/xsync/v4"
)

func newHub() *hub {
	return &hub{
		hub: xsync.NewMap[string, *Room](),
	}
}

func newRoom(name string) *Room {
	return &Room{
		room: xsync.NewMap[uint, *Subscriber](),
		name: name,
	}
}

func (r *Room) Join(sub *Subscriber) error {
	_, ok := r.room.Load(sub.fd)
	if ok {
		return SubscirberAlreadyInRoom
	}

	r.room.Store(sub.fd, sub)

	sub.mu.Lock()
	defer sub.mu.Unlock()
	if i := slices.Index(sub.rooms, r.name); i < 0 {
		sub.rooms = append(sub.rooms, r.name)
	}
	return nil
}

func (r *Room) Remove(sub *Subscriber) {
	r.room.Delete(sub.fd)
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if i := slices.Index(sub.rooms, r.name); i >= 0 {
		sub.rooms = slices.Delete(sub.rooms, i, i+1)
	}
}

func (r *Room) Count() int {
	return r.room.Size()
}

func (r *Room) Members() []*Subscriber {
	s := make([]*Subscriber, r.Count())
	i := 0

	r.room.RangeRelaxed(func(_ uint, value *Subscriber) bool {
		s[i] = value
		i++
		return true
	})

	return s
}

func (r *Room) Send(msg []byte) {
	r.room.RangeRelaxed(func(k uint, s *Subscriber) bool {
		s.Write(msg)
		return true
	})
}

// we will have shards
// shards will be in between 1 - 8
// the shard is decided by the room name string
// after the shard is decided it will add the subsriber in the room
// and add the room in the subscriber as well
//
// need to make a write function for subsriber
