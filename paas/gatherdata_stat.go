// Copyright 2016 JD.com. All Rights Reserved.
//
// 20160826 version 1.0 By:zhangxuejiao

package paas

import (
	"time"
)

type NetInterfaceStats struct {
	// The name of the interface.
	//	Name string `json:"name"`
	// Cumulative count of bytes received.
	RxBytes uint64 `json:"rx_bytes"`
	// Cumulative count of packets received.
	RxPackets uint64 `json:"rx_packets"`
	// Cumulative count of receive errors encountered.
	RxErrors uint64 `json:"rx_errors"`
	// Cumulative count of bytes transmitted.
	TxBytes uint64 `json:"tx_bytes"`
	// Cumulative count of packets transmitted.
	TxPackets uint64 `json:"tx_packets"`
	// Cumulative count of transmit errors encountered.
	TxErrors uint64 `json:"tx_errors"`
}

type FsStats struct {
	//Device string `json:"device,omitempty"`
	// Number of bytes that can be consumed by the container on this filesystem.
	Limit uint64 `json:"limit,omitempty"`
	// Number of bytes that is consumed by the container on this filesystem.
	Usage uint64 `json:"usage,omitempty"`
	// This is the total number of reads completed successfully.
	ReadsCompleted uint64 `json:"reads_completed"`
	// This is the total number of writes completed successfully.
	WritesCompleted uint64 `json:"writes_completed"`

	// Number of milliseconds spent reading
	// This is the total number of milliseconds spent by all reads (as
	// measured from __make_request() to end_that_request_last()).
	ReadTime uint64 `json:"read_time"`
	// Number of milliseconds spent writing
	// This is the total number of milliseconds spent by all writes (as
	// measured from __make_request() to end_that_request_last()).
	WriteTime uint64 `json:"write_time"`

	// Number of sectors read
	// This is the total number of sectors read successfully.
	SectorsRead uint64 `json:"sectors_read"`
	// Number of sectors written
	// This is the total number of sectors written successfully.
	SectorsWritten uint64 `json:"sectors_written"`

	// Number of I/Os currently in progres
	// The only field that should go to zero. Incremented as requests are
	// given to appropriate struct request_queue and decremented as they finish.
	IoInProgress uint64 `json:"io_in_progress"`
}

type DiskIOStats struct {
	//	DiskUtil       int32  `json:"disk util,omitempty"`
	DiskReadCount  uint64 `json:"disk_read_count,omitempty"`
	DiskWriteCount uint64 `json:"disk_write_count,omitempty"`
	DiskReadKB     uint64 `json:"disk_read_KB,omitempty"`
	DiskWriteKB    uint64 `json:"disk_write_KB,omitempty"`
}

type MiddleStats struct {
	//uuid
	Name string
	IP   string
	// The time of this stat point.
	Timestamp time.Time `json:"timestamp,omitempty"` // ContainerStats.Timestamp
	// Unit: nanoseconds.
	CpuTotal uint64 `json:"cpu_total,omitempty"` // ContainerStats.CpuStats.CpuUsage.Total
	// Load is smoothed over the last 10 seconds.
	LoadAverage int32 `json:"load_average,omitempty"` // ContainerStats.CpuStats.LoadAverage
	// bytes
	MemLimit  uint64 `json:"mem_total,omitempty"`
	MemUsed   uint64 `json:"mem_used,omitempty"`
	SwapLimit uint64 `json:"swap_total,omitempty"`
	SwapUsed  uint64 `json:"swap_used,omitempty"`
	//Count of TCP connections in state "Established"
	TcpEstablished uint64 `json:"tcp_established,omitempty"` // ContainerStats.NetworkStats.TcpStat.Established

	//NetItem map[string]int64    `json:"netitem,omitempty"`
	IpReceived     int64                        `json:" ip_package_received,omitempty"`
	IpDiscarded    int64                        `json:" ip_package_discarded,omitempty"`
	TcpReceived    int64                        `json:" tcp_package_received,omitempty"`
	TcpSendout     int64                        `json:" tcp_package_send out,omitempty"`
	TcpActiveOpen  int64                        `json:" tcp_active_open,omitempty"`
	TcpBadSegments int64                        `json:" tcp_bad_segments,omitempty"`
	Network        map[string]NetInterfaceStats `json:"network,omitempty"`
	FileSystem     map[string]FsStats           `json:"filesystem,omitempty"`
	//Network        []NetInterfaceStats `json:"network,omitempty"`
	//FileSystem []FsStats `json:"filesystem,omitempty"`
	//DiskIO         []DiskIOStats       `json:"disk io,omitempty"`
}

type GatherInterfaceStats struct {
	// The name of the interface.
	//Name string `json:"name"`
	// Cumulative count of KB per second received.
	RxKBPerSec int32 `json:"rx_kb_persec"`
	//RxPacketsPerSec int32 `json:"rx_packets_persec"`
	RxPackets      int32 `json:"rx_packets"`
	RxErrorsPerSec int32 `json:"rx_errors_persec"`
	// Cumulative count of KB per second transmitted.
	TxKBPerSec int32 `json:"tx_kb_persec"`
	// Cumulative count of packets transmitted.
	TxPackets int32 `json:"tx_packets"`
	//TxPacketsPerSec int32 `json:"tx_packets_persec"`
	// Cumulative count of transmit errors encountered.
	TxErrorsPerSec int32 `json:"tx_errors_persec"`
}

type GatherFsStats struct {
	//Device string `json:"device,omitempty"`
	//The value is multiplying by 1000
	UsagePercent int32 `json:"usage_percent,omitempty"`
	// read count after last gather
	ReadCount int32 `json:"reads_count"`
	// Write count after last gather
	WriteCount int32 `json:"writes_count"`
	// read bytes per second.
	ReadBytesPerSec int32 `json:"read_bytes_persec"`
	// Write bytes per second.
	WriteBytesPerSec int32 `json:"write_bytes_persec"`
	// Number of I/Os currently in progres
	// The only field that should go to zero. Incremented as requests are
	// given to appropriate struct request_queue and decremented as they finish.
	IoInProgress int32 `json:"io_in_progress"`
}

type GatherStats struct {
	//uuid
	Name string
	IP   string
	// The time of this stat point.
	Timestamp int64 `json:"timestamp,omitempty"` // need to tranlate from type time.Time to int64
	// The value has multified by 1000.
	CpuPercent int32 `json:"cpu_total_percent,omitempty"` // need to compute with CpuTotal
	// Load is smoothed over the last 10 seconds.
	LoadAverage int32 `json:"load_average,omitempty"`         // ContainerStats.CpuStats.LoadAverage
	MemPercent  int32 `json:"memory_usage_percent,omitempty"` // need to compute with CpuTotal
	SwapPercent int32 `json:"swap_usage_percent,omitempty"`   // need to compute with CpuTotal
	//Count of TCP connections in state "Established"
	TcpEstablished       uint64                          `json:"tcp_established,omitempty"` // ContainerStats.NetworkStats.TcpStat.Established
	IpReceivedPerSec     int64                           `json:"ip_package_received_per_second,omitempty"`
	IpDiscardedPerSec    int64                           `json:"ip_package_discarded_per_second,omitempty"`
	TcpReceivedPerSec    int64                           `json:"tcp_package_received_per_second,omitempty"`
	TcpSendoutPerSec     int64                           `json:"tcp_package_sendout_per_second,omitempty"`
	TcpActiveOpenPerSec  int64                           `json:"tcp_active_open_per_second,omitempty"`
	TcpBadSegmentsPerSec int64                           `json:"tcp_bad_segments_per_second,omitempty"`
	Network              map[string]GatherInterfaceStats `json:"network,omitempty"`
	FileSystem           map[string]GatherFsStats        `json:"file_system,omitempty"`
	//Network              []GatherInterfaceStats `json:"network,omitempty"`
	//FileSystem []FsStats           `json:"disk capacity,omitempty"`
	//Disk    []DiskCapacity      `json:"disk capacity,omitempty"`
	//	DiskIO  []DiskIOStats       `json:"disk io,omitempty"`
}
