# ChitoSocket
## _Less Memory Footprint Socket io_

*⚠️ Only works in linux*

ChitoSocket is a fast, lightweight WebSockets library for Go. It can handle millions of connections and uses very little RAM. ChitoSocket is easy to use, and you can write a blazingly fast WebSockets application in less than 10 lines of code. 
ChitoSocket is built on top of the [gobwas/ws](https://github.com/gobwas/ws) library. It is also based on the findings of the article "[Million WebSockets and Go](https://www.freecodecamp.org/news/million-websockets-and-go-cc58418460bb/)" by FreeCodeCamp.
ChitoSocket is still under development, you can see the examples in the [ChitoSocket Example](https://github.com/sairash/chitosocket-example) GitHub repository to learn how to use it.

*Example Project*
- https://github.com/sairash/chitosocket-example

*✨Links✨*
- https://github.com/gobwas/ws
- https://www.freecodecamp.org/news/million-websockets-and-go-cc58418460bb/
- https://github.com/sairash/chitosocket-example

*✨Chitosocket Packages✨*
| Langauge | Package Link |
| ------ | ------ |
| EcmaScript/ JS | https://www.npmjs.com/package/@sairash/chitosocket |
|  | * *New Packages Coming Soon* * |


## Making, Listening and Emiting

##### Upgrade connection: 
```go
conn, readWritter, subscruber, err := chitosocket.UpgradeConnection(c.Request(), c.Response())
```

##### Send data to room
```go
 chitosocket.Emit("message", room, op, data)
```

##### Listen to Event from client
```go
chitosocket.On["message"] = func(subs *chitosocket.Subscriber, op ws.OpCode, data map[string]interface{}) {
    chitosocket.Emit("message", room, op, data)
}
```
That's it!!

## License
MIT
