package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTailReader(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "amazon-ssm-agent.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	reader, err := NewTailReader(tmpFile.Name())
	require.NoError(t, err)
	bs, err := os.ReadFile("testdata/amazon-ssm-agent.log")
	require.NoError(t, err)
	tmpFile.Write(bs)
	tmpFile.Sync()
	actual := make([]byte, len(bs))
	n, err := reader.Read(actual)
	require.NoError(t, err)
	require.Equal(t, len(bs), n)
	require.EqualValues(t, bs, actual)
}
