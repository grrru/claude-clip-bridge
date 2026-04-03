package bridge

import (
	"bytes"
	"testing"
)

var testToken = [TokenSize]byte{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31, 32,
}

func TestProtocolRoundTrip(t *testing.T) {
	t.Parallel()

	var request bytes.Buffer
	if err := WritePNGRequest(&request, testToken); err != nil {
		t.Fatalf("WritePNGRequest() error = %v", err)
	}

	gotRequest, gotToken, err := ReadRequest(&request)
	if err != nil {
		t.Fatalf("ReadRequest() error = %v", err)
	}
	if gotRequest != RequestPNG {
		t.Fatalf("ReadRequest() = 0x%02x, want 0x%02x", gotRequest, RequestPNG)
	}
	if gotToken != testToken {
		t.Fatalf("ReadRequest() token mismatch")
	}

	payload := []byte("png-data")
	var response bytes.Buffer
	if err := WriteResponse(&response, payload); err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	gotPayload, err := ReadResponse(&response)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("ReadResponse() = %q, want %q", gotPayload, payload)
	}
}

func TestReadResponseZeroLength(t *testing.T) {
	t.Parallel()

	var response bytes.Buffer
	if err := WriteResponse(&response, nil); err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	gotPayload, err := ReadResponse(&response)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if gotPayload != nil {
		t.Fatalf("ReadResponse() = %v, want nil", gotPayload)
	}
}
