package chitosocket

import "fmt"

var (
	ConnectionNoExposeSyscallConn = fmt.Errorf("connection doesn not expose syscall.conn")
	SubscirberAlreadyInRoom       = fmt.Errorf("subscriber is already in room")
	RoomNotAvailable              = fmt.Errorf("room is not available")
)
