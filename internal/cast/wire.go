package cast

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const maxFrame = 1024 * 1024

type envelope struct {
	Source, Destination, Namespace string
	Payload                        []byte
}

func encode(e envelope) []byte {
	b := []byte{8, 0}
	for _, f := range []struct {
		tag  byte
		data []byte
	}{{18, []byte(e.Source)}, {26, []byte(e.Destination)}, {34, []byte(e.Namespace)}} {
		b = append(b, f.tag)
		b = binary.AppendUvarint(b, uint64(len(f.data)))
		b = append(b, f.data...)
	}
	b = append(b, 40, 0, 50)
	b = binary.AppendUvarint(b, uint64(len(e.Payload)))
	return append(b, e.Payload...)
}
func decode(b []byte) (envelope, error) {
	var e envelope
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return e, errors.New("invalid field")
		}
		b = b[n:]
		if key>>3 == 0 {
			return e, errors.New("invalid field number")
		}
		switch key & 7 {
		case 0:
			_, n = binary.Uvarint(b)
			if n <= 0 {
				return e, errors.New("invalid varint")
			}
			b = b[n:]
		case 2:
			size, n := binary.Uvarint(b)
			if n <= 0 {
				return e, errors.New("invalid length")
			}
			b = b[n:]
			if size > uint64(len(b)) {
				return e, io.ErrUnexpectedEOF
			}
			v := b[:int(size)]
			b = b[int(size):]
			switch key >> 3 {
			case 2:
				e.Source = string(v)
			case 3:
				e.Destination = string(v)
			case 4:
				e.Namespace = string(v)
			case 6:
				e.Payload = append([]byte(nil), v...)
			}
		case 1:
			if len(b) < 8 {
				return e, io.ErrUnexpectedEOF
			}
			b = b[8:]
		case 5:
			if len(b) < 4 {
				return e, io.ErrUnexpectedEOF
			}
			b = b[4:]
		default:
			return e, fmt.Errorf("unsupported wire type %d", key&7)
		}
	}
	return e, nil
}
func readEnvelope(r io.Reader) (envelope, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return envelope{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxFrame {
		return envelope{}, errors.New("invalid Cast frame size")
	}
	b := make([]byte, int(size))
	if _, err := io.ReadFull(r, b); err != nil {
		return envelope{}, err
	}
	return decode(b)
}
