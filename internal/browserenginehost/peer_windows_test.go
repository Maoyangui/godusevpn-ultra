//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"encoding/binary"
	"testing"
)

func syntheticTCPRow(localPort, remotePort, pid uint32) []byte {
	row := make([]byte, 24)
	values := []uint32{5, 0x0100007f, localPort, 0x0100007f, remotePort, pid}
	for i, v := range values {
		binary.LittleEndian.PutUint32(row[i*4:], v)
	}
	return row
}
func tcpPort(port uint16) uint32 { return uint32(port&255)<<8 | uint32(port>>8) }
func tcpRows(rows ...[]byte) []byte {
	table := make([]byte, 4)
	binary.LittleEndian.PutUint32(table, uint32(len(rows)))
	for _, row := range rows {
		table = append(table, row...)
	}
	return table
}
func TestReverseTupleOwnerUsesExactDirectionAndNetworkPort(t *testing.T) {
	row := syntheticTCPRow(tcpPort(50000), tcpPort(49152), 4321)
	if pid, err := reverseTuplePID(tcpRows(row), 50000, 49152); err != nil || pid != 4321 {
		t.Fatal("valid reverse owner tuple refused")
	}
	for _, table := range [][]byte{tcpRows(row, row), tcpRows(syntheticTCPRow(tcpPort(49152), tcpPort(50000), 123)), tcpRows(syntheticTCPRow(tcpPort(50000), tcpPort(49152), 0)), tcpRows(syntheticTCPRow(1<<24|tcpPort(50000), tcpPort(49152), 123)), {2, 0, 0, 0}, nil} {
		if _, err := reverseTuplePID(table, 50000, 49152); err == nil {
			t.Fatal("ambiguous/malformed/wrong-direction tuple accepted")
		}
	}
	for _, offset := range []int{0, 4, 12} {
		bad := append([]byte(nil), row...)
		binary.LittleEndian.PutUint32(bad[offset:], 0)
		if _, err := reverseTuplePID(tcpRows(bad), 50000, 49152); err == nil {
			t.Fatal("non-established or non-loopback owner accepted")
		}
	}
}
