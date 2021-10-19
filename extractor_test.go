package extractor

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/types"
)

func TestIndexBlock(t *testing.T) {
	examples := []struct {
		input    types.EventDataNewBlock
		expected string
		err      string
	}{
		{
			input: types.EventDataNewBlock{},
			err:   "nil Block",
		},
		{
			input: types.EventDataNewBlock{
				Block: &types.Block{
					Header: types.Header{
						ChainID: "chain-id",
						Height:  1,
						Time:    time.Unix(1634674166, 0),
					},
				},
			},
			expected: "DMLOG BLOCK 1 1634674166000 ChoKABIIY2hhaW4taWQYASIGCPbLvIsGKgISABIAGgA=\n",
		},
	}

	for _, ex := range examples {
		output := bytes.NewBuffer(nil)
		lock := &sync.Mutex{}

		err := indexBlock(output, lock, ex.input)
		if err != nil {
			assert.Equal(t, err.Error(), ex.err)
		}
		assert.Equal(t, ex.expected, output.String())
	}
}

func TestIndexTx(t *testing.T) {
	examples := []struct {
		input    *abci.TxResult
		expected string
		err      string
	}{
		{
			input:    &abci.TxResult{},
			expected: "DMLOG TX 0 0 IgA=\n",
		},
		{
			input: &abci.TxResult{
				Index:  0,
				Height: 1000,
				Tx:     []byte("data"),
			},
			expected: "DMLOG TX 1000 0 COgHGgRkYXRhIgA=\n",
		},
	}

	for _, ex := range examples {
		output := bytes.NewBuffer(nil)
		lock := &sync.Mutex{}

		err := indexTX(output, lock, ex.input)
		if err != nil {
			assert.Equal(t, err.Error(), ex.err)
		}
		assert.Equal(t, ex.expected, output.String())
	}
}

func TestFormatFilename(t *testing.T) {
	examples := []struct {
		input    string
		expected string
	}{
		{"file", "file"},
		{"file-$date", "file-" + time.Now().UTC().Format("20060102")},
		{"file-$time", "file-" + time.Now().UTC().Format("150405")},
		{"file-$ts", "file-" + time.Now().UTC().Format("20060102-150405")},
	}

	for _, ex := range examples {
		assert.Equal(t, ex.expected, formatFilename(ex.input))
	}
}

func TestAttributesString(t *testing.T) {
	attrs := []abci.EventAttribute{
		{Key: []byte("key1"), Value: []byte("value1")},
		{Key: []byte("key2"), Value: []byte("value2")},
		{Key: []byte("key3"), Value: []byte("value3")},
	}

	assert.Equal(t, "@@key1:value1@@key2:value2@@key3:value3", attributesString(attrs))
}
