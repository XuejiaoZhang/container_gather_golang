package paas

import (
	"os"
	"testing"
)

type FileArray struct {
	filename string
	ret      bool
}

type GatherDataArray struct {
	gatherstats []GatherStats
	err         error
}

func CreateGatherStatsData() []GatherStats {
	var gatherstats []GatherStats
	network_root := make(map[string]GatherInterfaceStats)
	network_root["eth0"] = GatherInterfaceStats{
		RxKBPerSec:     6,
		RxPackets:      97,
		RxErrorsPerSec: 0,
		TxKBPerSec:     63,
		TxPackets:      195,
		TxErrorsPerSec: 0,
	}
	fs_root := make(map[string]GatherFsStats)
	fs_root["/dev/sda2"] = GatherFsStats{
		UsagePercent:     274,
		ReadCount:        0,
		WriteCount:       0,
		ReadBytesPerSec:  0,
		WriteBytesPerSec: 0,
		IoInProgress:     0,
	}
	gatherstat_root := GatherStats{
		Name:        "/",
		IP:          "10.1.25.1 ",
		Timestamp:   1474441620659073709,
		CpuPercent:  67894,
		LoadAverage: 1233,
		MemPercent:  69,
		//SwapPercent: ,
		//TcpEstablished: ,
		IpReceivedPerSec: 21,
		//IpDiscardedPerSec: ,
		TcpReceivedPerSec: 21,
		TcpSendoutPerSec:  41,
		//TcpActiveOpenPerSec ,
		//TcpBadSegmentsPerSec: ,
		Network:    network_root,
		FileSystem: fs_root,
	}
	gatherstats = append(gatherstats, gatherstat_root)

	network_one := make(map[string]GatherInterfaceStats)
	network_one["eth0"] = GatherInterfaceStats{
		RxKBPerSec:     0,
		RxPackets:      0,
		RxErrorsPerSec: 0,
		TxKBPerSec:     0,
		TxPackets:      0,
		TxErrorsPerSec: 0,
	}
	fs_one := make(map[string]GatherFsStats)
	fs_one["docker-8:6-412989-pool"] = GatherFsStats{
		UsagePercent:     -1,
		ReadCount:        0,
		WriteCount:       0,
		ReadBytesPerSec:  0,
		WriteBytesPerSec: 0,
		IoInProgress:     0,
	}
	gatherstat_one := GatherStats{
		Name:       "/docker/57954f4873c04b155bf92d8367985c6df6de2faa0227cf35cca91fb046ef6712",
		IP:         "10.1.25.4",
		Timestamp:  1474441500691075464,
		CpuPercent: 3319,
		//LoadAverage: ,
		MemPercent:  799,
		SwapPercent: 186,
		//TcpEstablished: ,
		//IpReceivedPerSec: ,
		//IpDiscardedPerSec: ,
		//TcpReceivedPerSec: ,
		//TcpSendoutPerSec:  ,
		//TcpActiveOpenPerSec ,
		//TcpBadSegmentsPerSec: ,
		Network:    network_one,
		FileSystem: fs_one,
	}
	gatherstats = append(gatherstats, gatherstat_one)

	network_two := make(map[string]GatherInterfaceStats)
	network_two["eth0"] = GatherInterfaceStats{
		RxKBPerSec:     0,
		RxPackets:      0,
		RxErrorsPerSec: 0,
		TxKBPerSec:     0,
		TxPackets:      0,
		TxErrorsPerSec: 0,
	}
	fs_two := make(map[string]GatherFsStats)
	fs_two["docker-8:6-412989-pool"] = GatherFsStats{
		UsagePercent:     -1,
		ReadCount:        0,
		WriteCount:       0,
		ReadBytesPerSec:  0,
		WriteBytesPerSec: 0,
		IoInProgress:     0,
	}
	gatherstat_two := GatherStats{
		Name:       "/docker/aab0d1475fd5491201ff0ddeec7e4a7ae103f2a386be5f1dec4474f12b637b20",
		IP:         "10.1.25.5",
		Timestamp:  1474441505704362880,
		CpuPercent: 3180,
		//LoadAverage: ,
		MemPercent:  -1,
		SwapPercent: -1,
		//TcpEstablished: ,
		//IpReceivedPerSec: ,
		//IpDiscardedPerSec: ,
		//TcpReceivedPerSec: ,
		//TcpSendoutPerSec:  ,
		//TcpActiveOpenPerSec ,
		//TcpBadSegmentsPerSec: ,
		Network:    network_two,
		FileSystem: fs_two,
	}
	gatherstats = append(gatherstats, gatherstat_two)

	return gatherstats
}

func TestExist(t *testing.T) {
	var test_data = [3]FileArray{
		{"/root", true},
		{"./", true},
		{"1234567890qwertyuiop0987654321", false},
	}
	for _, v := range test_data {
		if v.ret != Exist(v.filename) {
			t.Errorf("Exist(%s) != %t \n", v.filename, v.ret)
		}
	}
}

func TestStoreIntoLocalFile(t *testing.T) {
	var gatherstats_null []GatherStats
	gatherstats := CreateGatherStatsData()
	var test_data = [2]GatherDataArray{
		{gatherstats_null, nil},
		{gatherstats, nil},
	}
	filename := "abctestabc.txt"
	file, _ := os.OpenFile(filename, os.O_CREATE, 0666)
	defer file.Close()
	for _, v := range test_data {
		if msg, err := StoreIntoLocalFile(v.gatherstats, filename); err != v.err {
			t.Errorf("StoreIntoLocalFile(%s) != %t \n", v.gatherstats, v.err, msg)
		} else {
			os.Remove(filename)
		}
	}
}
