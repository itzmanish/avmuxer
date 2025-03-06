package avmuxer

import (
	"errors"
	"log"
	"sync"

	"gopkg.in/hraban/opus.v2"
)

var int16BufferPool sync.Pool

type Multiplexer struct {
	sync.RWMutex
	encoder Encoder

	sources map[string]Stream
}

type Stream interface {
	ReadPCM([]int16) (int, error)
	WritePCM([]int16) (int, error)
}

type DecodingStream interface {
	Decode([]byte, []int16) (int, error)
}

type EncodingStream interface {
	Encode([]int16, []byte) (int, error)
}

type Encoder interface {
	Encode([]int16, []byte) (int, error)
	SampleSize() int
	ChannelCount() int
}

type Decoder interface {
	Decode([]byte, []int16) (int, error)
}

type OpusDecoder struct {
	od     *opus.Decoder
	buffer *RingBuffer[int16]
}

func (od *OpusDecoder) Decode(in []byte, out []int16) (int, error) {
	return od.od.Decode(in, out)
}

func NewOpusDecoder(sampleRate, channel, size int) (Decoder, error) {
	decoder, err := opus.NewDecoder(sampleRate, channel)
	if err != nil {
		return nil, err
	}
	return &OpusDecoder{
		od:     decoder,
		buffer: NewRingBuffer[int16](size * channel),
	}, nil
}

type OpusEncoder struct {
	size    int
	channel int

	oe     *opus.Encoder
	buffer *RingBuffer[byte]
}

func (oe *OpusEncoder) SampleSize() int {
	return oe.size
}

func (oe *OpusEncoder) ChannelCount() int {
	return oe.channel
}

func (oe *OpusEncoder) Encode(in []int16, out []byte) (int, error) {
	return oe.oe.Encode(in, out)
}

func NewOpusEncoder(sampleRate, channel, size int) (Encoder, error) {
	enc, err := opus.NewEncoder(sampleRate, channel, opus.AppAudio)
	if err != nil {
		return nil, err
	}
	err = enc.SetBitrate(int(opus.Fullband))
	if err != nil {
		return nil, err
	}
	return &OpusEncoder{
		size:    size,
		channel: channel,
		oe:      enc,
		buffer:  NewRingBuffer[byte](size * channel * 2), // int16 data holds 2 byte, size is sample size
	}, nil
}

func NewMultiplexer() *Multiplexer {
	return &Multiplexer{
		sources: make(map[string]Stream),
	}
}

func (mr *Multiplexer) AddEncoder(id string, enc Encoder) error {
	if mr.encoder != nil {
		return errors.New("encoder already configured")
	}

	mr.Lock()
	mr.encoder = enc
	mr.Unlock()
	return nil
}

// size should be calculated as clock_rate*sample_duration_in_ms/1000
func (mr *Multiplexer) AddSourceStream(id string, stream Stream) error {
	mr.Lock()
	defer mr.Unlock()
	if _, ok := mr.sources[id]; ok {
		return errors.New("stream already exists")
	}

	mr.sources[id] = stream
	return nil
}
func (mr *Multiplexer) RemoveSourceStream(id string) error {
	mr.Lock()
	defer mr.Unlock()
	delete(mr.sources, id)
	return nil
}

func (mr *Multiplexer) interleavedMultiplex(sampleSize int) []int16 {
	buffs := [][]int16{}
	maxBufSize := 0
	mr.RLock()
	for _, s := range mr.sources {
		bufI := int16BufferPool.Get()
		var buf []int16
		if bufI == nil {
			buf = make([]int16, sampleSize)
		} else {
			buf = *bufI.(*[]int16)
		}
		n, err := s.ReadPCM(buf)
		if err != nil {
			continue
		}
		buf = buf[:n]
		if n > maxBufSize {
			maxBufSize = n
		}
		buffs = append(buffs, buf)
	}
	mr.RUnlock()
	size := len(buffs)
	out := make([]int16, maxBufSize)
	// for i := maxBufSize - 1; i >= 0; i-- {
	for i := 0; i < maxBufSize; i++ {
		var sum int16
		for _, buf := range buffs {
			sum += (buf[i] / int16(size))
		}
		out[i] = sum
	}
	for _, buf := range buffs {
		if cap(buf) < 8184 {
			buf = buf[:0]
			int16BufferPool.Put(&buf)
		}
	}
	return out
}

func (mr *Multiplexer) WritePCM([]int16) (int, error) {
	return 0, errors.New("multiplexer stream doesn't support write method")
}

func (mr *Multiplexer) ReadPCM(sampleSize int) []int16 {
	return mr.interleavedMultiplex(sampleSize)
}

func (mr *Multiplexer) Read(dst []byte) (int, error) {
	data := mr.ReadPCM(mr.encoder.SampleSize() * mr.encoder.ChannelCount())
	if len(data) == 0 {
		return 0, nil
	}

	n, err := mr.encoder.Encode(data, dst)
	if err != nil {
		return 0, err
	}
	log.Printf("encoded data: size(pcm): %v, n: %v, slicecap: %v", len(data), n, len(dst))
	return n, nil
}
