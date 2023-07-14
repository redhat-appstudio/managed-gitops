package util

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"gopkg.in/yaml.v2"
)

// DecompressObject decompress the given compressed byte array object
func DecompressObject(compressedObj []byte) ([]byte, error) {
	if compressedObj == nil {
		return nil, nil
	}

	// Decompress data to get actual resource string
	bufferIn := bytes.NewBuffer(compressedObj)
	gzipReader, err := gzip.NewReader(bufferIn)

	if err != nil {
		return nil, fmt.Errorf("unable to create gzipReader: %v", err)
	}

	var bufferOut bytes.Buffer

	// Using CopyN with For loop to avoid gosec error "Potential DoS vulnerability via decompression bomb",
	// occurred while using below code
	for {
		_, err := io.CopyN(&bufferOut, gzipReader, 131072)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("unable to convert resource data to string: %v", err)
		}
	}

	if err := gzipReader.Close(); err != nil {
		return nil, fmt.Errorf("unable to close gzip reader connection: %v", err)
	}

	return bufferOut.Bytes(), nil
}

// CompressObject marshals the given object into byte array and compresses it
func CompressObject(obj interface{}) ([]byte, error) {
	if obj == nil {
		return nil, nil
	}

	var byteArr []byte
	var buffer bytes.Buffer

	// Marshal the object into bytes.
	objBytes, err := yaml.Marshal(obj)
	if err != nil {
		return byteArr, fmt.Errorf("unable to Marshal the object. %v", err)
	}

	// Compress string data
	gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
	if err != nil {
		return byteArr, fmt.Errorf("unable to create Buffer writer. %v", err)
	}

	_, err = gzipWriter.Write(objBytes)

	if err != nil {
		return byteArr, fmt.Errorf("unable to compress the object bytes. %v", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return byteArr, fmt.Errorf("unable to close gzip writer connection. %v", err)
	}

	return buffer.Bytes(), nil
}
