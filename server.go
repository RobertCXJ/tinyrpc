// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tinyrpc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
	"tinyrpc/codec"
)

// ------------------------------
/*
这段代码定义了一个 Option 结构体类型，它包含两个字段：MagicNumber 和 CodecType。
其中，MagicNumber 字段用于标识这是一个 geerpc 请求，它的值为一个固定的常量 0x3bef5c，用于校验请求的合法性。
CodecType 字段表示客户端可以选择不同的编解码器来对请求体进行编解码。
此外，这段代码还定义了一个名为 DefaultOption 的全局变量，它的值为一个指向 Option 类型的指针。
DefaultOption 变量被初始化为一个具有默认值的 Option 结构体，其中的 MagicNumber 和 CodecType 字段分别被赋值为 0x3bef5c 和 GobType。
这个变量通常用于为 Server 或 Client 提供默认配置，如果用户没有显式地指定配置，则会使用这个默认值。
在使用 Option 结构体类型时，我们可以使用默认值，也可以根据需要创建自定义的配置。
*/

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int        // MagicNumber marks this's a geerpc request
	CodecType   codec.Type // client may choose different Codec to encode body
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

// ------------------------------

// Server represents an RPC Server.
type Server struct{}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

// --------------------------
/*
这段代码是一个 RPC 服务端实现的一部分，它定义了一个名为 ServeConn 的方法。
ServeConn 方法接收一个 io.ReadWriteCloser 接口类型的参数 conn，表示连接到客户端的网络连接。
在方法中，首先使用 json.NewDecoder 将 conn 对象解码为 Option 结构体类型的对象 opt，并检查解码过程中是否出现错误。
Option 结构体中包含了 RPC 协议相关的一些选项，如编解码方式、魔数等。
接下来，代码会检查解码得到的 opt 对象中的魔数是否与预定义的魔数相等。
如果不相等，则表示客户端发来的消息不合法，应该直接返回。否则，继续执行下一步。
代码中定义了一个名为 f 的变量，该变量的值为一个函数类型，其输入参数为 io.ReadWriteCloser 类型，输出为 Codec 接口类型。
Codec 接口是 RPC 协议中编解码器的接口类型，具体的编解码实现可以使用不同的库或者自己实现。
在这里，根据 opt 对象中指定的编解码方式，从 NewCodecFuncMap 映射表中获取相应的编解码器构造函数，并将 conn 对象传递给该函数，
以获取一个 Codec 接口类型的对象。如果映射表中找不到对应的构造函数，就直接返回。
最后，调用 server.serveCodec 方法，并将上一步得到的 Codec 对象作为参数传递给该方法。
serveCodec 方法将使用该编解码器对象来处理 RPC 请求和响应。整个 ServeConn 方法的执行过程中还使用了 defer 语句，在方法执行完毕后会关闭连接 conn。
*/

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	server.serveCodec(f(conn))
}

// --------------------------

// invalidRequest is a placeholder for response argv when error occurs
var invalidRequest = struct{}{}

func (server *Server) serveCodec(cc codec.Codec) {
	sending := new(sync.Mutex) // make sure to send a complete response
	wg := new(sync.WaitGroup)  // wait until all request are handled
	for {
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break // it's not possible to recover, so close the connection
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()
}

// request stores all information of a call
type request struct {
	h            *codec.Header // header of request
	argv, replyv reflect.Value // argv and replyv of request
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	// TODO: now we don't know the type of request argv
	// day 1, just suppose it's string
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
		log.Println("rpc server: read argv err:", err)
	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// TODO, should call registered rpc methods to get the right replyv
	// day 1, just print argv and send a hello message
	defer wg.Done()
	log.Println(req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.h.Seq))
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go server.ServeConn(conn)
	}
}

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func Accept(lis net.Listener) { DefaultServer.Accept(lis) }
