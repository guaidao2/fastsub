package verify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"

	"github.com/guaidao2/fastsub/internal/model"
)

// faviconLimit is how much of an icon is read. Icons are small, and one that is
// not is not an icon.
const faviconLimit = 512 << 10

// faviconHash fetches /favicon.ico and returns its fingerprint, computed the
// way Shodan and FOFA compute it: the base64 of the icon, through MurmurHash3,
// as a signed 32-bit integer.
//
// The point of a favicon hash is that it belongs to the application rather than
// the host. Two names with nothing in common — different addresses, different
// certificates, different titles — answer with the same number when the same
// product serves both, which is a relationship no DNS record contains.
func (p *Prober) faviconHash(ctx context.Context, base model.URL) (int32, bool) {
	address := base.URL + "favicon.ico"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", p.opts.UserAgent)

	resp, err := p.client().Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false
	}

	icon, err := io.ReadAll(io.LimitReader(resp.Body, faviconLimit))
	if err != nil || len(icon) == 0 {
		return 0, false
	}
	// A site that answers every path with its home page is not serving an icon,
	// and hashing that page would produce a fingerprint of the 404 handler.
	if ct := resp.Header.Get("Content-Type"); ct != "" && !isIconType(ct) && !looksLikeIcon(icon) {
		return 0, false
	}

	return murmur3_32(base64WithNewlines(icon), 0), true
}

func (p *Prober) client() *http.Client {
	if p.opts.Redirects {
		return p.follow
	}
	return p.noFollow
}

func isIconType(contentType string) bool {
	for _, want := range []string{"image/", "application/octet-stream", "application/x-icon"} {
		if len(contentType) >= len(want) && contentType[:len(want)] == want {
			return true
		}
	}
	return false
}

// looksLikeIcon checks the magic numbers of the formats a favicon actually
// comes in. Content-Type from a default server configuration is not evidence.
func looksLikeIcon(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch {
	case data[0] == 0x00 && data[1] == 0x00 && (data[2] == 0x01 || data[2] == 0x02):
		return true // .ico
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return true
	case bytes.HasPrefix(data, []byte("GIF8")):
		return true
	case bytes.HasPrefix(data, []byte("<svg")) || bytes.HasPrefix(data, []byte("<?xml")):
		return true
	case data[0] == 0xFF && data[1] == 0xD8:
		return true // JPEG
	}
	return false
}

// base64WithNewlines encodes the way Python's base64.encodebytes does: wrapped
// at 76 columns with a trailing newline. The wrapping is part of the input to
// the hash, so not doing it produces a number that matches nothing elsewhere.
func base64WithNewlines(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)

	var buf bytes.Buffer
	buf.Grow(len(encoded) + len(encoded)/76 + 1)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		buf.WriteString(encoded[i:end])
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// murmur3_32 is MurmurHash3 x86 32-bit. It is here rather than in a dependency
// because it is the whole of what a favicon hash needs, and the spelling has to
// match the one everyone else uses exactly.
func murmur3_32(data []byte, seed uint32) int32 {
	const (
		c1 = 0xcc9e2d51
		c2 = 0x1b873593
	)

	var h1 uint32 = seed

	body := len(data) / 4 * 4
	for i := 0; i < body; i += 4 {
		k1 := binary.LittleEndian.Uint32(data[i:])

		k1 *= c1
		k1 = k1<<15 | k1>>17
		k1 *= c2

		h1 ^= k1
		h1 = h1<<13 | h1>>19
		h1 = h1*5 + 0xe6546b64
	}

	var k1 uint32
	switch tail := data[body:]; len(tail) {
	case 3:
		k1 ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(tail[0])
		k1 *= c1
		k1 = k1<<15 | k1>>17
		k1 *= c2
		h1 ^= k1
	}

	h1 ^= uint32(len(data))
	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return int32(h1)
}
