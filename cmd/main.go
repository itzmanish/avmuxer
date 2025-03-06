package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/pion/webrtc/v4/pkg/media/oggreader"
	"gopkg.in/hraban/opus.v2"
)

// Helper function to convert int16 PCM data to byte slice
func Int16ToByteSlice(samples []int16) []byte {
	byteSlice := make([]byte, len(samples)*2)
	for i, sample := range samples {
		byteSlice[i*2] = byte(sample)
		byteSlice[i*2+1] = byte(sample >> 8)
	}
	return byteSlice
}

// Helper function to convert byte slice to int16 PCM data
func ByteSliceToInt16(samples []byte) []int16 {
	pcm := make([]int16, len(samples)/2)
	for i := 0; i < len(samples); i += 2 {
		pcm[i/2] = int16(samples[i]) | int16(samples[i+1])<<8
	}
	return pcm
}

func main() {
	// Open the input OGG file
	inputFile, err := os.Open("3.ogg")
	if err != nil {
		log.Fatalf("Error opening input file: %v", err)
	}
	defer inputFile.Close()

	// Create a new Opus decoder
	clockRate := 48000 // 48 kHz
	channels := 2      // Stereo
	decoder, err := opus.NewDecoder(clockRate, channels)
	if err != nil {
		log.Fatalln("failed to init decoder", err)
	}

	// Create a buffer to hold the decoded PCM data
	var pcmBuffer bytes.Buffer
	// Decode the Opus data from the OGG file
	oggReader, _, err := oggreader.NewWith(inputFile)
	if err != nil {
		log.Fatalf("Error initializing ogg reader: %w", err)
	}
	var lastGranule uint64 = 0
	for {
		opusData, header, err := oggReader.ParseNextPage()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				log.Fatalf("Error reading ogg data: %w", err)
			}
		}
		sampleCount := float64(header.GranulePosition - lastGranule)
		lastGranule = header.GranulePosition
		sampleDuration := time.Duration((sampleCount/float64(clockRate))*1000) * time.Millisecond

		samples := uint64(sampleDuration.Seconds() * float64(clockRate))
		log.Printf("samples: %v, sampleCount: %v, duration: %v, header: %+v, raw: %v", samples, sampleCount, sampleDuration, header, opusData)
		if sampleCount == 0 {
			continue
		}
		pcm := make([]int16, 960*channels)
		n, err := decoder.Decode(opusData, pcm)
		if err != nil {
			log.Fatalf("Error decoding Opus data: %v", err)
		}
		log.Printf("n: %v, pcm: %v, len(pcm): %v", n, pcm, len(pcm))
		pcmBytes := Int16ToByteSlice(pcm[:n*channels])
		pcmBuffer.Write(pcmBytes)
	}

	// // Create a new Opus encoder
	// encoder, err := opus.NewEncoder(clockRate, channels, opus.AppAudio)
	// if err != nil {
	// 	log.Fatalf("Error creating Opus encoder: %v", err)
	// }

	// Create the output OGG file
	outputFile, err := os.Create("3.pcm")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer outputFile.Close()

	// Encode the PCM data back to Opus and write to the OGG file
	pcmData := pcmBuffer.Bytes()

	n, err := outputFile.Write(pcmData)
	if err != nil {
		log.Fatalf("failed to write pcm data to file: %w", err)
	}
	log.Println("saved: %v bytes", n)
	// pcmSamples := ByteSliceToInt16(pcmData)
	// for len(pcmSamples) > 0 {
	// 	frameSize := 960
	// 	if len(pcmSamples) < frameSize*channels {
	// 		frameSize = len(pcmSamples) / channels
	// 	}
	// 	opusData := make([]byte, 4000)
	// 	n, err := encoder.Encode(pcmSamples[:frameSize*channels], opusData)
	// 	if err != nil {
	// 		log.Fatalf("Error encoding Opus data: %v", err)
	// 	}
	// 	opusData = opusData[:n]
	// 	oggWriter.WritePacket(opusData, 0, len(opusData))
	// 	pcmSamples = pcmSamples[frameSize*channels:]
	// }

	fmt.Println("Conversion complete!")
}
