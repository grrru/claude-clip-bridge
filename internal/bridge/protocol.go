package bridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const RequestPNG byte = 0x01
const TokenSize = 32
const maxPayloadSize = 50 * 1024 * 1024 // 50MB

var ErrUnsupportedRequest = errors.New("unsupported request type")

func WritePNGRequest(w io.Writer, token [TokenSize]byte) error {
	var buf [1 + TokenSize]byte
	buf[0] = RequestPNG
	copy(buf[1:], token[:])
	_, err := w.Write(buf[:])
	return err
}

func ReadRequest(r io.Reader) (byte, [TokenSize]byte, error) {
	var buf [1 + TokenSize]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, [TokenSize]byte{}, err
	}

	var token [TokenSize]byte
	copy(token[:], buf[1:])
	return buf[0], token, nil
}

func WriteResponse(w io.Writer, payload []byte) error {
	if len(payload) > int(^uint32(0)) {
		return fmt.Errorf("payload too large: %d", len(payload))
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}

	if len(payload) == 0 {
		return nil
	}

	_, err := w.Write(payload)
	return err
}

func ReadResponse(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header[:])
	if length == 0 {
		return nil, nil
	}

	if int(length) > maxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	return payload, nil
}
