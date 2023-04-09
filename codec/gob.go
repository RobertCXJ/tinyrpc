package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

/*
对于 RPC 协议来说，这部分协商是需要自主设计的。为了提升性能，一般在报文的最开始会规划固定的字节，来协商相关的信息。
*/

// --------------------------
/**
Go 语言中，变量赋值表达式左侧如果有一个下划线（_）标识符，那么该标识符就会被称为一个空标识符，并被视为“不需要使用的变量”。
在这段代码中，var _ Codec = (*GobCodec)(nil) 定义了一个类型为 Codec 的变量，并将其赋值为 (*GobCodec)(nil)，
其中 (*GobCodec)(nil) 表示一个 GobCodec 类型的空指针。这个变量的名称是一个空标识符 _。
这个变量的作用是为了确保 GobCodec 结构体类型实现了 Codec 接口类型的所有方法。
如果 GobCodec 结构体类型没有实现 Codec 接口类型的所有方法，编译器就会报错。
因此，这个变量在代码中并没有直接用到，只是用来检查接口实现的正确性，可以用任何名称代替 _。
总之，var _ Codec = (*GobCodec)(nil) 的作用是用来检查 GobCodec 结构体类型是否实现了 Codec 接口类型的所有方法。
*/

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

// --------------------------

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(h); err != nil {
		log.Println("rpc: gob error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err != nil {
		log.Println("rpc: gob error encoding body:", err)
		return
	}
	return
}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}
