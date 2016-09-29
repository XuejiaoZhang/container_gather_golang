// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Manager of cAdvisor-monitored containers.
package manager

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/cadvisor/cache/memory"
	"github.com/google/cadvisor/collector"
	"github.com/google/cadvisor/container"
	"github.com/google/cadvisor/container/docker"
	"github.com/google/cadvisor/container/raw"
	"github.com/google/cadvisor/container/rkt"
	"github.com/google/cadvisor/container/systemd"
	"github.com/google/cadvisor/events"
	"github.com/google/cadvisor/fs"
	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/info/v2"
	"github.com/google/cadvisor/machine"
	"github.com/google/cadvisor/manager/watcher"
	rawwatcher "github.com/google/cadvisor/manager/watcher/raw"
	rktwatcher "github.com/google/cadvisor/manager/watcher/rkt"
	"github.com/google/cadvisor/utils/oomparser"
	"github.com/google/cadvisor/utils/sysfs"
	"github.com/google/cadvisor/version"

	"net/http"

	"github.com/golang/glog"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	//paas start
	"github.com/google/cadvisor/paas"
	//paas end
)

var globalHousekeepingInterval = flag.Duration("global_housekeeping_interval", 1*time.Minute, "Interval between global housekeepings")
var logCadvisorUsage = flag.Bool("log_cadvisor_usage", false, "Whether to log the usage of the cAdvisor container")
var eventStorageAgeLimit = flag.String("event_storage_age_limit", "default=24h", "Max length of time for which to store events (per type). Value is a comma separated list of key values, where the keys are event types (e.g.: creation, oom) or \"default\" and the value is a duration. Default is applied to all non-specified event types")
var eventStorageEventLimit = flag.String("event_storage_event_limit", "default=100000", "Max number of events to store (per type). Value is a comma separated list of key values, where the keys are event types (e.g.: creation, oom) or \"default\" and the value is an integer. Default is applied to all non-specified event types")
var applicationMetricsCountLimit = flag.Int("application_metrics_count_limit", 100, "Max number of application metrics to store (per container)")

//paas start
/*
var collectHousekeepingInterval = 1 * time.Minute
var localFileHousekeepingInterval = 2 * time.Minute
var localfile = "gather_data.txt"
*/
var collectHousekeepingInterval = flag.Duration("collect_housekeeping_interval", 1*time.Minute, "Interval between collect housekeepings")
var localFileHousekeepingInterval = flag.Duration("scan_localfile_housekeeping_interval", 2*time.Minute, "Interval between scan local file housekeepings")
var localfile = flag.String("stored_localfile", "gather_data.txt", "name of local file which stores data failed to gather")

//paas end

// The Manager interface defines operations for starting a manager and getting
// container and machine information.
type Manager interface {
	// Start the manager. Calling other manager methods before this returns
	// may produce undefined behavior.
	Start() error

	// Stops the manager.
	Stop() error

	//  information about a container.
	GetContainerInfo(containerName string, query *info.ContainerInfoRequest) (*info.ContainerInfo, error)

	// Get V2 information about a container.
	// Recursive (subcontainer) requests are best-effort, and may return a partial result alongside an
	// error in the partial failure case.
	GetContainerInfoV2(containerName string, options v2.RequestOptions) (map[string]v2.ContainerInfo, error)

	// Get information about all subcontainers of the specified container (includes self).
	SubcontainersInfo(containerName string, query *info.ContainerInfoRequest) ([]*info.ContainerInfo, error)

	// Gets all the Docker containers. Return is a map from full container name to ContainerInfo.
	AllDockerContainers(query *info.ContainerInfoRequest) (map[string]info.ContainerInfo, error)

	// Gets information about a specific Docker container. The specified name is within the Docker namespace.
	DockerContainer(dockerName string, query *info.ContainerInfoRequest) (info.ContainerInfo, error)

	// Gets spec for all containers based on request options.
	GetContainerSpec(containerName string, options v2.RequestOptions) (map[string]v2.ContainerSpec, error)

	// Gets summary stats for all containers based on request options.
	GetDerivedStats(containerName string, options v2.RequestOptions) (map[string]v2.DerivedStats, error)

	// Get info for all requested containers based on the request options.
	GetRequestedContainersInfo(containerName string, options v2.RequestOptions) (map[string]*info.ContainerInfo, error)

	// Returns true if the named container exists.
	Exists(containerName string) bool

	// Get information about the machine.
	GetMachineInfo() (*info.MachineInfo, error)

	// Get version information about different components we depend on.
	GetVersionInfo() (*info.VersionInfo, error)

	// Get filesystem information for a given label.
	// Returns information for all global filesystems if label is empty.
	GetFsInfo(label string) ([]v2.FsInfo, error)

	// Get ps output for a container.
	GetProcessList(containerName string, options v2.RequestOptions) ([]v2.ProcessInfo, error)

	// Get events streamed through passedChannel that fit the request.
	WatchForEvents(request *events.Request) (*events.EventChannel, error)

	// Get past events that have been detected and that fit the request.
	GetPastEvents(request *events.Request) ([]*info.Event, error)

	CloseEventChannel(watch_id int)

	// Get status information about docker.
	DockerInfo() (info.DockerStatus, error)

	// Get details about interesting docker images.
	DockerImages() ([]info.DockerImage, error)

	// Returns debugging information. Map of lines per category.
	DebugInfo() map[string][]string

	//paas start
	//collect data from memory every 1 min
	CollectHousekeeping(containerManager Manager) ([]byte, error)
	LocalFileHousekeeping(containerManager Manager) ([]byte, error)
	//paas end
}

// New takes a memory storage and returns a new manager.
func New(memoryCache *memory.InMemoryCache, sysfs sysfs.SysFs, maxHousekeepingInterval time.Duration, allowDynamicHousekeeping bool, ignoreMetricsSet container.MetricSet, collectorHttpClient *http.Client) (Manager, error) {
	if memoryCache == nil {
		return nil, fmt.Errorf("manager requires memory storage")
	}

	// Detect the container we are running on.
	selfContainer, err := cgroups.GetThisCgroupDir("cpu")
	if err != nil {
		return nil, err
	}
	glog.Infof("cAdvisor running in container: %q", selfContainer)

	dockerStatus, err := docker.Status()
	if err != nil {
		glog.Warningf("Unable to connect to Docker: %v", err)
	}
	rktPath, err := rkt.RktPath()
	if err != nil {
		glog.Warningf("unable to connect to Rkt api service: %v", err)
	}

	context := fs.Context{
		Docker: fs.DockerContext{
			Root:         docker.RootDir(),
			Driver:       dockerStatus.Driver,
			DriverStatus: dockerStatus.DriverStatus,
		},
		RktPath: rktPath,
	}
	fsInfo, err := fs.NewFsInfo(context)
	if err != nil {
		return nil, err
	}

	// If cAdvisor was started with host's rootfs mounted, assume that its running
	// in its own namespaces.
	inHostNamespace := false
	if _, err := os.Stat("/rootfs/proc"); os.IsNotExist(err) {
		inHostNamespace = true
	}

	// Register for new subcontainers.
	eventsChannel := make(chan watcher.ContainerEvent, 16)

	newManager := &manager{
		containers:               make(map[namespacedContainerName]*containerData),
		quitChannels:             make([]chan error, 0, 2),
		memoryCache:              memoryCache,
		fsInfo:                   fsInfo,
		cadvisorContainer:        selfContainer,
		inHostNamespace:          inHostNamespace,
		startupTime:              time.Now(),
		maxHousekeepingInterval:  maxHousekeepingInterval,
		allowDynamicHousekeeping: allowDynamicHousekeeping,
		ignoreMetrics:            ignoreMetricsSet,
		containerWatchers:        []watcher.ContainerWatcher{},
		eventsChannel:            eventsChannel,
		collectorHttpClient:      collectorHttpClient,
	}
	glog.Infof("manager: %+v", newManager)

	machineInfo, err := machine.Info(sysfs, fsInfo, inHostNamespace)
	if err != nil {
		return nil, err
	}
	newManager.machineInfo = *machineInfo
	glog.Infof("Machine: %+v", newManager.machineInfo)

	versionInfo, err := getVersionInfo()
	if err != nil {
		return nil, err
	}
	glog.Infof("Version: %+v", *versionInfo)

	newManager.eventHandler = events.NewEventManager(parseEventsStoragePolicy())
	return newManager, nil
}

// A namespaced container name.
type namespacedContainerName struct {
	// The namespace of the container. Can be empty for the root namespace.
	Namespace string

	// The name of the container in this namespace.
	Name string
}

type manager struct {
	containers               map[namespacedContainerName]*containerData
	containersLock           sync.RWMutex
	memoryCache              *memory.InMemoryCache
	fsInfo                   fs.FsInfo
	machineInfo              info.MachineInfo
	quitChannels             []chan error
	cadvisorContainer        string
	inHostNamespace          bool
	eventHandler             events.EventManager
	startupTime              time.Time
	maxHousekeepingInterval  time.Duration
	allowDynamicHousekeeping bool
	ignoreMetrics            container.MetricSet
	containerWatchers        []watcher.ContainerWatcher
	eventsChannel            chan watcher.ContainerEvent
	collectorHttpClient      *http.Client
}

// Start the container manager.
func (self *manager) Start() error {
	err := docker.Register(self, self.fsInfo, self.ignoreMetrics)
	if err != nil {
		glog.Warningf("Docker container factory registration failed: %v.", err)
	}

	err = rkt.Register(self, self.fsInfo, self.ignoreMetrics)
	if err != nil {
		glog.Warningf("Registration of the rkt container factory failed: %v", err)
	} else {
		watcher, err := rktwatcher.NewRktContainerWatcher()
		if err != nil {
			return err
		}
		self.containerWatchers = append(self.containerWatchers, watcher)
	}

	err = systemd.Register(self, self.fsInfo, self.ignoreMetrics)
	if err != nil {
		glog.Warningf("Registration of the systemd container factory failed: %v", err)
	}

	err = raw.Register(self, self.fsInfo, self.ignoreMetrics)
	if err != nil {
		glog.Errorf("Registration of the raw container factory failed: %v", err)
	}

	rawWatcher, err := rawwatcher.NewRawContainerWatcher()
	if err != nil {
		return err
	}
	self.containerWatchers = append(self.containerWatchers, rawWatcher)

	// Watch for OOMs.
	err = self.watchForNewOoms()
	if err != nil {
		glog.Warningf("Could not configure a source for OOM detection, disabling OOM events: %v", err)
	}

	// If there are no factories, don't start any housekeeping and serve the information we do have.
	if !container.HasFactories() {
		return nil
	}

	// Create root and then recover all containers.
	err = self.createContainer("/", watcher.Raw)
	if err != nil {
		return err
	}
	glog.Infof("Starting recovery of all containers")
	err = self.detectSubcontainers("/")
	if err != nil {
		return err
	}
	glog.Infof("Recovery completed")

	// Watch for new container.
	quitWatcher := make(chan error)
	err = self.watchForNewContainers(quitWatcher)
	if err != nil {
		return err
	}
	self.quitChannels = append(self.quitChannels, quitWatcher)

	// Look for new containers in the main housekeeping thread.
	quitGlobalHousekeeping := make(chan error)
	self.quitChannels = append(self.quitChannels, quitGlobalHousekeeping)
	go self.globalHousekeeping(quitGlobalHousekeeping)

	return nil
}

func (self *manager) Stop() error {
	// Stop and wait on all quit channels.
	for i, c := range self.quitChannels {
		// Send the exit signal and wait on the thread to exit (by closing the channel).
		c <- nil
		err := <-c
		if err != nil {
			// Remove the channels that quit successfully.
			self.quitChannels = self.quitChannels[i:]
			return err
		}
	}
	self.quitChannels = make([]chan error, 0, 2)
	return nil
}

func (self *manager) globalHousekeeping(quit chan error) {
	// Long housekeeping is either 100ms or half of the housekeeping interval.
	longHousekeeping := 100 * time.Millisecond
	if *globalHousekeepingInterval/2 < longHousekeeping {
		longHousekeeping = *globalHousekeepingInterval / 2
	}

	ticker := time.Tick(*globalHousekeepingInterval)
	for {
		select {
		case t := <-ticker:
			start := time.Now()

			// Check for new containers.
			err := self.detectSubcontainers("/")
			if err != nil {
				glog.Errorf("Failed to detect containers: %s", err)
			}

			// Log if housekeeping took too long.
			duration := time.Since(start)
			if duration >= longHousekeeping {
				glog.V(3).Infof("Global Housekeeping(%d) took %s", t.Unix(), duration)
			}
		case <-quit:
			// Quit if asked to do so.
			quit <- nil
			glog.Infof("Exiting global housekeeping thread")
			return
		}
	}
}

func (self *manager) getContainerData(containerName string) (*containerData, error) {
	var cont *containerData
	var ok bool
	func() {
		self.containersLock.RLock()
		defer self.containersLock.RUnlock()

		// Ensure we have the container.
		cont, ok = self.containers[namespacedContainerName{
			Name: containerName,
		}]
	}()
	if !ok {
		return nil, fmt.Errorf("unknown container %q", containerName)
	}
	return cont, nil
}

func (self *manager) GetDerivedStats(containerName string, options v2.RequestOptions) (map[string]v2.DerivedStats, error) {
	conts, err := self.getRequestedContainers(containerName, options)
	if err != nil {
		return nil, err
	}
	var errs partialFailure
	stats := make(map[string]v2.DerivedStats)
	for name, cont := range conts {
		d, err := cont.DerivedStats()
		if err != nil {
			errs.append(name, "DerivedStats", err)
		}
		stats[name] = d
	}
	return stats, errs.OrNil()
}

func (self *manager) GetContainerSpec(containerName string, options v2.RequestOptions) (map[string]v2.ContainerSpec, error) {
	conts, err := self.getRequestedContainers(containerName, options)
	if err != nil {
		return nil, err
	}
	var errs partialFailure
	specs := make(map[string]v2.ContainerSpec)
	for name, cont := range conts {
		cinfo, err := cont.GetInfo()
		if err != nil {
			errs.append(name, "GetInfo", err)
		}
		spec := self.getV2Spec(cinfo)
		specs[name] = spec
	}
	return specs, errs.OrNil()
}

// Get V2 container spec from v1 container info.
func (self *manager) getV2Spec(cinfo *containerInfo) v2.ContainerSpec {
	spec := self.getAdjustedSpec(cinfo)
	return v2.ContainerSpecFromV1(&spec, cinfo.Aliases, cinfo.Namespace)
}

func (self *manager) getAdjustedSpec(cinfo *containerInfo) info.ContainerSpec {
	spec := cinfo.Spec

	// Set default value to an actual value
	if spec.HasMemory {
		// Memory.Limit is 0 means there's no limit
		if spec.Memory.Limit == 0 {
			spec.Memory.Limit = uint64(self.machineInfo.MemoryCapacity)
		}
	}
	return spec
}

func (self *manager) GetContainerInfo(containerName string, query *info.ContainerInfoRequest) (*info.ContainerInfo, error) {
	cont, err := self.getContainerData(containerName)
	if err != nil {
		return nil, err
	}
	return self.containerDataToContainerInfo(cont, query)
}

func (self *manager) GetContainerInfoV2(containerName string, options v2.RequestOptions) (map[string]v2.ContainerInfo, error) {
	containers, err := self.getRequestedContainers(containerName, options)
	if err != nil {
		return nil, err
	}

	var errs partialFailure
	var nilTime time.Time // Ignored.

	infos := make(map[string]v2.ContainerInfo, len(containers))
	for name, container := range containers {
		result := v2.ContainerInfo{}
		cinfo, err := container.GetInfo()
		if err != nil {
			errs.append(name, "GetInfo", err)
			infos[name] = result
			continue
		}
		result.Spec = self.getV2Spec(cinfo)

		stats, err := self.memoryCache.RecentStats(name, nilTime, nilTime, options.Count)
		if err != nil {
			errs.append(name, "RecentStats", err)
			infos[name] = result
			continue
		}

		result.Stats = v2.ContainerStatsFromV1(&cinfo.Spec, stats)
		infos[name] = result
	}

	return infos, errs.OrNil()
}

func (self *manager) containerDataToContainerInfo(cont *containerData, query *info.ContainerInfoRequest) (*info.ContainerInfo, error) {
	// Get the info from the container.
	cinfo, err := cont.GetInfo()
	if err != nil {
		return nil, err
	}

	stats, err := self.memoryCache.RecentStats(cinfo.Name, query.Start, query.End, query.NumStats)
	if err != nil {
		return nil, err
	}
	//glog.Infof("memory stats : %p\n", stats)
	// Make a copy of the info for the user.
	ret := &info.ContainerInfo{
		ContainerReference: cinfo.ContainerReference,
		Subcontainers:      cinfo.Subcontainers,
		Spec:               self.getAdjustedSpec(cinfo),
		Stats:              stats,
	}
	return ret, nil
}

func (self *manager) getContainer(containerName string) (*containerData, error) {
	self.containersLock.RLock()
	defer self.containersLock.RUnlock()
	cont, ok := self.containers[namespacedContainerName{Name: containerName}]
	if !ok {
		return nil, fmt.Errorf("unknown container %q", containerName)
	}
	return cont, nil
}

func (self *manager) getSubcontainers(containerName string) map[string]*containerData {
	self.containersLock.RLock()
	defer self.containersLock.RUnlock()
	containersMap := make(map[string]*containerData, len(self.containers))

	// Get all the unique subcontainers of the specified container
	matchedName := path.Join(containerName, "/")
	for i := range self.containers {
		name := self.containers[i].info.Name
		if name == containerName || strings.HasPrefix(name, matchedName) {
			containersMap[self.containers[i].info.Name] = self.containers[i]
		}
	}
	return containersMap
}

func (self *manager) SubcontainersInfo(containerName string, query *info.ContainerInfoRequest) ([]*info.ContainerInfo, error) {
	containersMap := self.getSubcontainers(containerName)

	containers := make([]*containerData, 0, len(containersMap))
	for _, cont := range containersMap {
		containers = append(containers, cont)
	}
	return self.containerDataSliceToContainerInfoSlice(containers, query)
}

func (self *manager) getAllDockerContainers() map[string]*containerData {
	self.containersLock.RLock()
	defer self.containersLock.RUnlock()
	containers := make(map[string]*containerData, len(self.containers))

	// Get containers in the Docker namespace.
	for name, cont := range self.containers {
		if name.Namespace == docker.DockerNamespace {
			containers[cont.info.Name] = cont
		}
	}
	return containers
}

func (self *manager) AllDockerContainers(query *info.ContainerInfoRequest) (map[string]info.ContainerInfo, error) {
	containers := self.getAllDockerContainers()

	output := make(map[string]info.ContainerInfo, len(containers))
	for name, cont := range containers {
		inf, err := self.containerDataToContainerInfo(cont, query)
		if err != nil {
			return nil, err
		}
		output[name] = *inf
	}
	return output, nil
}

func (self *manager) getDockerContainer(containerName string) (*containerData, error) {
	self.containersLock.RLock()
	defer self.containersLock.RUnlock()

	// Check for the container in the Docker container namespace.
	cont, ok := self.containers[namespacedContainerName{
		Namespace: docker.DockerNamespace,
		Name:      containerName,
	}]
	if !ok {
		return nil, fmt.Errorf("unable to find Docker container %q", containerName)
	}
	return cont, nil
}

func (self *manager) DockerContainer(containerName string, query *info.ContainerInfoRequest) (info.ContainerInfo, error) {
	container, err := self.getDockerContainer(containerName)
	if err != nil {
		return info.ContainerInfo{}, err
	}

	inf, err := self.containerDataToContainerInfo(container, query)
	if err != nil {
		return info.ContainerInfo{}, err
	}
	return *inf, nil
}

func (self *manager) containerDataSliceToContainerInfoSlice(containers []*containerData, query *info.ContainerInfoRequest) ([]*info.ContainerInfo, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found")
	}

	// Get the info for each container.
	output := make([]*info.ContainerInfo, 0, len(containers))
	for i := range containers {
		cinfo, err := self.containerDataToContainerInfo(containers[i], query)
		if err != nil {
			// Skip containers with errors, we try to degrade gracefully.
			continue
		}
		output = append(output, cinfo)
	}

	return output, nil
}

func (self *manager) GetRequestedContainersInfo(containerName string, options v2.RequestOptions) (map[string]*info.ContainerInfo, error) {
	containers, err := self.getRequestedContainers(containerName, options)
	if err != nil {
		return nil, err
	}
	var errs partialFailure
	containersMap := make(map[string]*info.ContainerInfo)
	query := info.ContainerInfoRequest{
		NumStats: options.Count,
	}
	for name, data := range containers {
		info, err := self.containerDataToContainerInfo(data, &query)
		if err != nil {
			errs.append(name, "containerDataToContainerInfo", err)
		}
		containersMap[name] = info
	}
	return containersMap, errs.OrNil()
}

func (self *manager) getRequestedContainers(containerName string, options v2.RequestOptions) (map[string]*containerData, error) {
	containersMap := make(map[string]*containerData)
	switch options.IdType {
	case v2.TypeName:
		if options.Recursive == false {
			cont, err := self.getContainer(containerName)
			if err != nil {
				return containersMap, err
			}
			containersMap[cont.info.Name] = cont
		} else {
			containersMap = self.getSubcontainers(containerName)
			if len(containersMap) == 0 {
				return containersMap, fmt.Errorf("unknown container: %q", containerName)
			}
		}
	case v2.TypeDocker:
		if options.Recursive == false {
			containerName = strings.TrimPrefix(containerName, "/")
			cont, err := self.getDockerContainer(containerName)
			if err != nil {
				return containersMap, err
			}
			containersMap[cont.info.Name] = cont
		} else {
			if containerName != "/" {
				return containersMap, fmt.Errorf("invalid request for docker container %q with subcontainers", containerName)
			}
			containersMap = self.getAllDockerContainers()
		}
	default:
		return containersMap, fmt.Errorf("invalid request type %q", options.IdType)
	}
	return containersMap, nil
}

func (self *manager) GetFsInfo(label string) ([]v2.FsInfo, error) {
	var empty time.Time
	// Get latest data from filesystems hanging off root container.
	stats, err := self.memoryCache.RecentStats("/", empty, empty, 1)
	if err != nil {
		return nil, err
	}
	dev := ""
	if len(label) != 0 {
		dev, err = self.fsInfo.GetDeviceForLabel(label)
		if err != nil {
			return nil, err
		}
	}
	fsInfo := []v2.FsInfo{}
	for i := range stats[0].Filesystem {
		fs := stats[0].Filesystem[i]
		if len(label) != 0 && fs.Device != dev {
			continue
		}
		mountpoint, err := self.fsInfo.GetMountpointForDevice(fs.Device)
		if err != nil {
			return nil, err
		}
		labels, err := self.fsInfo.GetLabelsForDevice(fs.Device)
		if err != nil {
			return nil, err
		}

		fi := v2.FsInfo{
			Device:     fs.Device,
			Mountpoint: mountpoint,
			Capacity:   fs.Limit,
			Usage:      fs.Usage,
			Available:  fs.Available,
			Labels:     labels,
		}
		if fs.HasInodes {
			fi.Inodes = &fs.Inodes
			fi.InodesFree = &fs.InodesFree
		}
		fsInfo = append(fsInfo, fi)
	}
	return fsInfo, nil
}

func (m *manager) GetMachineInfo() (*info.MachineInfo, error) {
	// Copy and return the MachineInfo.
	return &m.machineInfo, nil
}

func (m *manager) GetVersionInfo() (*info.VersionInfo, error) {
	// TODO: Consider caching this and periodically updating.  The VersionInfo may change if
	// the docker daemon is started after the cAdvisor client is created.  Caching the value
	// would be helpful so we would be able to return the last known docker version if
	// docker was down at the time of a query.
	return getVersionInfo()
}

func (m *manager) Exists(containerName string) bool {
	m.containersLock.Lock()
	defer m.containersLock.Unlock()

	namespacedName := namespacedContainerName{
		Name: containerName,
	}

	_, ok := m.containers[namespacedName]
	if ok {
		return true
	}
	return false
}

func (m *manager) GetProcessList(containerName string, options v2.RequestOptions) ([]v2.ProcessInfo, error) {
	// override recursive. Only support single container listing.
	options.Recursive = false
	conts, err := m.getRequestedContainers(containerName, options)
	if err != nil {
		return nil, err
	}
	if len(conts) != 1 {
		return nil, fmt.Errorf("Expected the request to match only one container")
	}
	// TODO(rjnagal): handle count? Only if we can do count by type (eg. top 5 cpu users)
	ps := []v2.ProcessInfo{}
	for _, cont := range conts {
		ps, err = cont.GetProcessList(m.cadvisorContainer, m.inHostNamespace)
		if err != nil {
			return nil, err
		}
	}
	return ps, nil
}

func (m *manager) registerCollectors(collectorConfigs map[string]string, cont *containerData) error {
	for k, v := range collectorConfigs {
		configFile, err := cont.ReadFile(v, m.inHostNamespace)
		if err != nil {
			return fmt.Errorf("failed to read config file %q for config %q, container %q: %v", k, v, cont.info.Name, err)
		}
		glog.V(3).Infof("Got config from %q: %q", v, configFile)

		if strings.HasPrefix(k, "prometheus") || strings.HasPrefix(k, "Prometheus") {
			newCollector, err := collector.NewPrometheusCollector(k, configFile, *applicationMetricsCountLimit, cont.handler, m.collectorHttpClient)
			if err != nil {
				glog.Infof("failed to create collector for container %q, config %q: %v", cont.info.Name, k, err)
				return err
			}
			err = cont.collectorManager.RegisterCollector(newCollector)
			if err != nil {
				glog.Infof("failed to register collector for container %q, config %q: %v", cont.info.Name, k, err)
				return err
			}
		} else {
			newCollector, err := collector.NewCollector(k, configFile, *applicationMetricsCountLimit, cont.handler, m.collectorHttpClient)
			if err != nil {
				glog.Infof("failed to create collector for container %q, config %q: %v", cont.info.Name, k, err)
				return err
			}
			err = cont.collectorManager.RegisterCollector(newCollector)
			if err != nil {
				glog.Infof("failed to register collector for container %q, config %q: %v", cont.info.Name, k, err)
				return err
			}
		}
	}
	return nil
}

// Enables overwriting an existing containerData/Handler object for a given containerName.
// Can't use createContainer as it just returns if a given containerName has a handler already.
// Ex: rkt handler will want to take priority over the raw handler, but the raw handler might be created first.

// Only allow raw handler to be overridden
func (m *manager) overrideContainer(containerName string, watchSource watcher.ContainerWatchSource) error {
	m.containersLock.Lock()
	defer m.containersLock.Unlock()

	namespacedName := namespacedContainerName{
		Name: containerName,
	}

	if _, ok := m.containers[namespacedName]; ok {
		containerData := m.containers[namespacedName]

		if containerData.handler.Type() != container.ContainerTypeRaw {
			return nil
		}

		err := m.destroyContainerLocked(containerName)
		if err != nil {
			return fmt.Errorf("overrideContainer: failed to destroy containerData/handler for %v: %v", containerName, err)
		}
	}

	return m.createContainerLocked(containerName, watchSource)
}

// Create a container.
func (m *manager) createContainer(containerName string, watchSource watcher.ContainerWatchSource) error {
	m.containersLock.Lock()
	defer m.containersLock.Unlock()

	return m.createContainerLocked(containerName, watchSource)
}

func (m *manager) createContainerLocked(containerName string, watchSource watcher.ContainerWatchSource) error {
	namespacedName := namespacedContainerName{
		Name: containerName,
	}

	// Check that the container didn't already exist.
	if _, ok := m.containers[namespacedName]; ok {
		return nil
	}

	handler, accept, err := container.NewContainerHandler(containerName, watchSource, m.inHostNamespace)
	if err != nil {
		return err
	}
	if !accept {
		// ignoring this container.
		glog.V(4).Infof("ignoring container %q", containerName)
		return nil
	}
	collectorManager, err := collector.NewCollectorManager()
	if err != nil {
		return err
	}

	logUsage := *logCadvisorUsage && containerName == m.cadvisorContainer
	cont, err := newContainerData(containerName, m.memoryCache, handler, logUsage, collectorManager, m.maxHousekeepingInterval, m.allowDynamicHousekeeping)
	if err != nil {
		return err
	}

	// Add collectors
	labels := handler.GetContainerLabels()
	collectorConfigs := collector.GetCollectorConfigs(labels)
	err = m.registerCollectors(collectorConfigs, cont)
	if err != nil {
		glog.Infof("failed to register collectors for %q: %v", containerName, err)
	}

	// Add the container name and all its aliases. The aliases must be within the namespace of the factory.
	m.containers[namespacedName] = cont
	for _, alias := range cont.info.Aliases {
		m.containers[namespacedContainerName{
			Namespace: cont.info.Namespace,
			Name:      alias,
		}] = cont
	}

	glog.V(3).Infof("Added container: %q (aliases: %v, namespace: %q)", containerName, cont.info.Aliases, cont.info.Namespace)

	contSpec, err := cont.handler.GetSpec()
	if err != nil {
		return err
	}

	contRef, err := cont.handler.ContainerReference()
	if err != nil {
		return err
	}

	newEvent := &info.Event{
		ContainerName: contRef.Name,
		Timestamp:     contSpec.CreationTime,
		EventType:     info.EventContainerCreation,
	}
	err = m.eventHandler.AddEvent(newEvent)
	if err != nil {
		return err
	}

	// Start the container's housekeeping.
	return cont.Start()
}

func (m *manager) destroyContainer(containerName string) error {
	m.containersLock.Lock()
	defer m.containersLock.Unlock()

	return m.destroyContainerLocked(containerName)
}

func (m *manager) destroyContainerLocked(containerName string) error {
	namespacedName := namespacedContainerName{
		Name: containerName,
	}
	cont, ok := m.containers[namespacedName]
	if !ok {
		// Already destroyed, done.
		return nil
	}

	// Tell the container to stop.
	err := cont.Stop()
	if err != nil {
		return err
	}

	// Remove the container from our records (and all its aliases).
	delete(m.containers, namespacedName)
	for _, alias := range cont.info.Aliases {
		delete(m.containers, namespacedContainerName{
			Namespace: cont.info.Namespace,
			Name:      alias,
		})
	}
	glog.V(3).Infof("Destroyed container: %q (aliases: %v, namespace: %q)", containerName, cont.info.Aliases, cont.info.Namespace)

	contRef, err := cont.handler.ContainerReference()
	if err != nil {
		return err
	}

	newEvent := &info.Event{
		ContainerName: contRef.Name,
		Timestamp:     time.Now(),
		EventType:     info.EventContainerDeletion,
	}
	err = m.eventHandler.AddEvent(newEvent)
	if err != nil {
		return err
	}
	return nil
}

// Detect all containers that have been added or deleted from the specified container.
func (m *manager) getContainersDiff(containerName string) (added []info.ContainerReference, removed []info.ContainerReference, err error) {
	m.containersLock.RLock()
	defer m.containersLock.RUnlock()

	// Get all subcontainers recursively.
	cont, ok := m.containers[namespacedContainerName{
		Name: containerName,
	}]
	if !ok {
		return nil, nil, fmt.Errorf("failed to find container %q while checking for new containers", containerName)
	}
	allContainers, err := cont.handler.ListContainers(container.ListRecursive)
	if err != nil {
		return nil, nil, err
	}
	allContainers = append(allContainers, info.ContainerReference{Name: containerName})

	// Determine which were added and which were removed.
	allContainersSet := make(map[string]*containerData)
	for name, d := range m.containers {
		// Only add the canonical name.
		if d.info.Name == name.Name {
			allContainersSet[name.Name] = d
		}
	}

	// Added containers
	for _, c := range allContainers {
		delete(allContainersSet, c.Name)
		_, ok := m.containers[namespacedContainerName{
			Name: c.Name,
		}]
		if !ok {
			added = append(added, c)
		}
	}

	// Removed ones are no longer in the container listing.
	for _, d := range allContainersSet {
		removed = append(removed, d.info.ContainerReference)
	}

	return
}

// Detect the existing subcontainers and reflect the setup here.
func (m *manager) detectSubcontainers(containerName string) error {
	added, removed, err := m.getContainersDiff(containerName)
	if err != nil {
		return err
	}

	// Add the new containers.
	for _, cont := range added {
		err = m.createContainer(cont.Name, watcher.Raw)
		if err != nil {
			glog.Errorf("Failed to create existing container: %s: %s", cont.Name, err)
		}
	}

	// Remove the old containers.
	for _, cont := range removed {
		err = m.destroyContainer(cont.Name)
		if err != nil {
			glog.Errorf("Failed to destroy existing container: %s: %s", cont.Name, err)
		}
	}

	return nil
}

// Watches for new containers started in the system. Runs forever unless there is a setup error.
func (self *manager) watchForNewContainers(quit chan error) error {
	for _, watcher := range self.containerWatchers {
		err := watcher.Start(self.eventsChannel)
		if err != nil {
			return err
		}
	}

	// There is a race between starting the watch and new container creation so we do a detection before we read new containers.
	err := self.detectSubcontainers("/")
	if err != nil {
		return err
	}

	// Listen to events from the container handler.
	go func() {
		for {
			select {
			case event := <-self.eventsChannel:
				switch {
				case event.EventType == watcher.ContainerAdd:
					switch event.WatchSource {
					// the Rkt and Raw watchers can race, and if Raw wins, we want Rkt to override and create a new handler for Rkt containers
					case watcher.Rkt:
						err = self.overrideContainer(event.Name, event.WatchSource)
					default:
						err = self.createContainer(event.Name, event.WatchSource)
					}
				case event.EventType == watcher.ContainerDelete:
					err = self.destroyContainer(event.Name)
				}
				if err != nil {
					glog.Warningf("Failed to process watch event %+v: %v", event, err)
				}
			case <-quit:
				var errs partialFailure

				// Stop processing events if asked to quit.
				for i, watcher := range self.containerWatchers {
					err := watcher.Stop()
					if err != nil {
						errs.append(fmt.Sprintf("watcher %d", i), "Stop", err)
					}
				}

				if len(errs) > 0 {
					quit <- errs
				} else {
					quit <- nil
					glog.Infof("Exiting thread watching subcontainers")
					return
				}
			}
		}
	}()
	return nil
}

func (self *manager) watchForNewOoms() error {
	glog.Infof("Started watching for new ooms in manager")
	outStream := make(chan *oomparser.OomInstance, 10)
	oomLog, err := oomparser.New()
	if err != nil {
		return err
	}
	go oomLog.StreamOoms(outStream)

	go func() {
		for oomInstance := range outStream {
			// Surface OOM and OOM kill events.
			newEvent := &info.Event{
				ContainerName: oomInstance.ContainerName,
				Timestamp:     oomInstance.TimeOfDeath,
				EventType:     info.EventOom,
			}
			err := self.eventHandler.AddEvent(newEvent)
			if err != nil {
				glog.Errorf("failed to add OOM event for %q: %v", oomInstance.ContainerName, err)
			}
			glog.V(3).Infof("Created an OOM event in container %q at %v", oomInstance.ContainerName, oomInstance.TimeOfDeath)

			newEvent = &info.Event{
				ContainerName: oomInstance.VictimContainerName,
				Timestamp:     oomInstance.TimeOfDeath,
				EventType:     info.EventOomKill,
				EventData: info.EventData{
					OomKill: &info.OomKillEventData{
						Pid:         oomInstance.Pid,
						ProcessName: oomInstance.ProcessName,
					},
				},
			}
			err = self.eventHandler.AddEvent(newEvent)
			if err != nil {
				glog.Errorf("failed to add OOM kill event for %q: %v", oomInstance.ContainerName, err)
			}
		}
	}()
	return nil
}

// can be called by the api which will take events returned on the channel
func (self *manager) WatchForEvents(request *events.Request) (*events.EventChannel, error) {
	return self.eventHandler.WatchEvents(request)
}

// can be called by the api which will return all events satisfying the request
func (self *manager) GetPastEvents(request *events.Request) ([]*info.Event, error) {
	return self.eventHandler.GetEvents(request)
}

// called by the api when a client is no longer listening to the channel
func (self *manager) CloseEventChannel(watch_id int) {
	self.eventHandler.StopWatch(watch_id)
}

// Parses the events StoragePolicy from the flags.
func parseEventsStoragePolicy() events.StoragePolicy {
	policy := events.DefaultStoragePolicy()

	// Parse max age.
	parts := strings.Split(*eventStorageAgeLimit, ",")
	for _, part := range parts {
		items := strings.Split(part, "=")
		if len(items) != 2 {
			glog.Warningf("Unknown event storage policy %q when parsing max age", part)
			continue
		}
		dur, err := time.ParseDuration(items[1])
		if err != nil {
			glog.Warningf("Unable to parse event max age duration %q: %v", items[1], err)
			continue
		}
		if items[0] == "default" {
			policy.DefaultMaxAge = dur
			continue
		}
		policy.PerTypeMaxAge[info.EventType(items[0])] = dur
	}

	// Parse max number.
	parts = strings.Split(*eventStorageEventLimit, ",")
	for _, part := range parts {
		items := strings.Split(part, "=")
		if len(items) != 2 {
			glog.Warningf("Unknown event storage policy %q when parsing max event limit", part)
			continue
		}
		val, err := strconv.Atoi(items[1])
		if err != nil {
			glog.Warningf("Unable to parse integer from %q: %v", items[1], err)
			continue
		}
		if items[0] == "default" {
			policy.DefaultMaxNumEvents = val
			continue
		}
		policy.PerTypeMaxNumEvents[info.EventType(items[0])] = val
	}

	return policy
}

func (m *manager) DockerImages() ([]info.DockerImage, error) {
	return docker.Images()
}

func (m *manager) DockerInfo() (info.DockerStatus, error) {
	return docker.Status()
}

func (m *manager) DebugInfo() map[string][]string {
	debugInfo := container.DebugInfo()

	// Get unique containers.
	var conts map[*containerData]struct{}
	func() {
		m.containersLock.RLock()
		defer m.containersLock.RUnlock()

		conts = make(map[*containerData]struct{}, len(m.containers))
		for _, c := range m.containers {
			conts[c] = struct{}{}
		}
	}()

	// List containers.
	lines := make([]string, 0, len(conts))
	for cont := range conts {
		lines = append(lines, cont.info.Name)
		if cont.info.Namespace != "" {
			lines = append(lines, fmt.Sprintf("\tNamespace: %s", cont.info.Namespace))
		}

		if len(cont.info.Aliases) != 0 {
			lines = append(lines, "\tAliases:")
			for _, alias := range cont.info.Aliases {
				lines = append(lines, fmt.Sprintf("\t\t%s", alias))
			}
		}
	}

	debugInfo["Managed containers"] = lines
	return debugInfo
}

func getVersionInfo() (*info.VersionInfo, error) {

	kernel_version := machine.KernelVersion()
	container_os := machine.ContainerOsVersion()
	docker_version := docker.VersionString()

	return &info.VersionInfo{
		KernelVersion:      kernel_version,
		ContainerOsVersion: container_os,
		DockerVersion:      docker_version,
		CadvisorVersion:    version.Info["version"],
		CadvisorRevision:   version.Info["revision"],
	}, nil
}

// Helper for accumulating partial failures.
type partialFailure []string

func (f *partialFailure) append(id, operation string, err error) {
	*f = append(*f, fmt.Sprintf("[%q: %s: %s]", id, operation, err))
}

func (f partialFailure) Error() string {
	return fmt.Sprintf("partial failures: %s", strings.Join(f, ", "))
}

func (f partialFailure) OrNil() error {
	if len(f) == 0 {
		return nil
	}
	return f
}

//paas start

// make sure collectHousekeepingInterval >= 2 x HousekeepingInterval
func CollectHousekeepingIntervalCheck(collectHousekeepingInterval *time.Duration, HousekeepingInterval *time.Duration) *time.Duration {
	if *collectHousekeepingInterval < 2**HousekeepingInterval {
		collectHousekeepingInterval_ret := 2 * *HousekeepingInterval
		return &collectHousekeepingInterval_ret
	} else {
		return collectHousekeepingInterval
	}
}

// make sure localFileHousekeepingInterval >= 2 x collectHousekeepingInterval & collectHousekeepingInterval >2min
func LocalFileHousekeepingIntervalCheck(localFileHousekeepingInterval *time.Duration, collectHousekeepingInterval *time.Duration) *time.Duration {
	min := time.Duration(2 * 60 * 1000 * 1000 * 1000)
	if 2**collectHousekeepingInterval > min {
		min = 2 * *collectHousekeepingInterval
	}
	if *localFileHousekeepingInterval < min {
		return &min
	}
	return localFileHousekeepingInterval
}

//check whether the dir of local file exists
func LocalFileDirCheck(localfile *string) *string {
	dir, _ := path.Split(*localfile)
	if dir == "" {
		return localfile
	} else {
		if !paas.Exist(dir) {
			os.MkdirAll(dir, 0666)
		}
		return localfile
	}

}

//report data
func (m *manager) CollectHousekeeping(containerManager Manager) ([]byte, error) {
	// colect housekeeping is 1 minnute.
	//collectHousekeeping := 1 * time.Minute
	//collectHousekeeping := 5 * time.Second
	// Housekeep every minute.
	glog.Infof("Start housekeeping for collecting data \n")
	fmt.Println("Start housekeeping for collecting data \n")
	collectHousekeepingInterval = CollectHousekeepingIntervalCheck(collectHousekeepingInterval, HousekeepingInterval)
	lastCollectHousekeeping := time.Now()
	for {
		select {
		//		case <-c.stop:
		// Stop housekeeping when signaled.
		//			return
		default:
			// Perform housekeeping.
			getMemoryStats(containerManager)
			// Log if housekeeping took too long.
			duration := time.Since(lastCollectHousekeeping)
			if duration >= *collectHousekeepingInterval {
				glog.V(3).Infof("Collect Housekeeping took %s", duration)
			}
		}
		// Log usage if asked to do so.
		/*
			if c.logUsage {

			}
		*/
		next := lastCollectHousekeeping.Add(*collectHousekeepingInterval)
		if time.Now().Before(next) {
			time.Sleep(next.Sub(time.Now()))
		} else {
			next = time.Now()
		}
		lastCollectHousekeeping = next
	}
}

func getMemoryStats(m Manager) error {
	start := time.Now()
	// The container name is the path after the handler
	containerName := "/"

	if containerName == "/" {
		// Get the containers.
		// collectHousekeepingInterval: 1min HousekeepingInterval: 5 sec , so 20 samples
		reqParams := info.ContainerInfoRequest{
			NumStats: int(*collectHousekeepingInterval)/int(*HousekeepingInterval) + 2,
		}
		// Get / info
		cont_root, err := m.GetContainerInfo(containerName, &reqParams)
		if err != nil {
			return fmt.Errorf("failed to get container %q with error: %v", containerName, err)
		}
		middlestats := CustomDataFormat(cont_root)
		var gatherstats []paas.GatherStats
		gatherstats = append(gatherstats, ComputeHandle(middlestats, start)...)

		// Get every container info
		conts, err := m.AllDockerContainers(&reqParams)
		if err != nil {
			return fmt.Errorf("failed to get container %q with error: %v", containerName, err)
		}
		for _, cont := range conts {
			//glog.Infof(" AllDockerContainers info name %s\n", cont.ContainerReference.Name)
			middlestats_container := CustomDataFormat(&cont)
			gatherstats = append(gatherstats, ComputeHandle(middlestats_container, start)...)
		}

		//Gather
		//go paas.GatherData(gatherstats)
		msg, err := paas.GatherData(gatherstats)
		if err != nil {
			glog.Infof("failed to gather data: ", err)
			//fmt.Println("failed to gather data: ", err)
			msg2, err2 := paas.StoreIntoLocalFile(gatherstats, *localfile)
			if err2 != nil {
				glog.Infof("failed to store data into local file: ", err2)
				fmt.Println("failed to store data into local file: ", err2)
			} else {
				fmt.Println("store data into local file: ", msg2)
			}
		} else {
			fmt.Println("gather data: ", msg)
		}
	}

	glog.V(5).Infof("Request took %s", time.Since(start))
	return nil
}

func getContainerDisplayName(cont info.ContainerReference) string {
	// Pick a user-added alias as display name.
	displayName := ""
	for _, alias := range cont.Aliases {
		// ignore container id as alias.
		if strings.Contains(cont.Name, alias) {
			continue
		}
		// pick shortest display name if multiple aliases are available.
		if displayName == "" || len(displayName) >= len(alias) {
			displayName = alias
		}
	}

	if displayName == "" {
		displayName = cont.Name
	} else if len(displayName) > 50 {
		// truncate display name to fit in one line.
		displayName = displayName[:50] + "..."
	}

	// Add the full container name to the display name.
	if displayName != cont.Name {
		displayName = fmt.Sprintf("%s (%s)", displayName, cont.Name)
	}

	return displayName
}

// format to necessary data
func CustomDataFormat(cont *info.ContainerInfo) []paas.MiddleStats {
	var middlestats []paas.MiddleStats
	for _, s := range cont.Stats {
		network := make(map[string]paas.NetInterfaceStats)
		for _, n := range s.Network.Interfaces {
			// only keep eth0 network data
			if n.Name == "eth0" {
				network[n.Name] = paas.NetInterfaceStats{n.RxBytes, n.RxPackets, n.RxErrors, n.TxBytes, n.TxPackets, n.TxErrors}
			}

		}
		filesystem := make(map[string]paas.FsStats)
		for _, f := range s.Filesystem {
			filesystem[f.Device] = paas.FsStats{f.Limit, f.Usage, f.ReadsCompleted, f.WritesCompleted, f.ReadTime, f.WriteTime, f.SectorsRead, f.SectorsWritten, f.IoInProgress}
		}
		//var filesystem []paas.FsStats
		//for _, f := range s.Filesystem {
		//	filesystem = append(filesystem, paas.FsStats{f.Device, f.Limit, f.Usage, f.ReadsCompleted, f.WritesCompleted, f.ReadTime, f.WriteTime, f.SectorsRead, f.SectorsWritten, f.IoInProgress})
		//}

		var ip string
		if value, ok := s.CustomMetrics["IP"]; ok {
			ip = value[0].StringValue
		}
		var ip_received int64
		ip_received = -1
		if value, ok := s.CustomMetrics["ip_received"]; ok {
			ip_received = value[0].IntValue
		}
		var ip_discarded int64
		ip_discarded = -1
		if value, ok := s.CustomMetrics["ip_discarded"]; ok {
			ip_discarded = value[0].IntValue
		}
		var tcp_received int64
		tcp_received = -1
		if value, ok := s.CustomMetrics["tcp_received"]; ok {
			tcp_received = value[0].IntValue
		}
		var tcp_sendout int64
		tcp_sendout = -1
		if value, ok := s.CustomMetrics["tcp_sendout"]; ok {
			tcp_sendout = value[0].IntValue
		}
		var tcp_activeopen int64
		tcp_activeopen = -1
		if value, ok := s.CustomMetrics["tcp_activeopen"]; ok {
			tcp_activeopen = value[0].IntValue
		}
		var tcp_bad_segments int64
		tcp_bad_segments = -1
		if value, ok := s.CustomMetrics["tcp_bad_segments"]; ok {
			tcp_bad_segments = value[0].IntValue
		}

		middle_stat := paas.MiddleStats{
			Name:           cont.ContainerReference.Name,
			IP:             ip,
			Timestamp:      s.Timestamp,
			CpuTotal:       s.Cpu.Usage.Total,
			LoadAverage:    s.Cpu.LoadAverage,
			MemLimit:       cont.Spec.Memory.Limit,
			MemUsed:        s.Memory.Usage,
			SwapLimit:      cont.Spec.Memory.SwapLimit,
			SwapUsed:       s.Memory.Swap,
			TcpEstablished: s.Network.Tcp.Established,
			IpReceived:     ip_received,
			IpDiscarded:    ip_discarded,
			TcpReceived:    tcp_received,
			TcpSendout:     tcp_sendout,
			TcpActiveOpen:  tcp_activeopen,
			TcpBadSegments: tcp_bad_segments,
			Network:        network,
			FileSystem:     filesystem,
		}
		//fmt.Println("middle_stat", middle_stat)
		middlestats = append(middlestats, middle_stat)
	}
	return middlestats
}

func ComputeHandle(middlestats []paas.MiddleStats, start time.Time) []paas.GatherStats {
	var gatherstats []paas.GatherStats
	for i := 0; i < len(middlestats)-1; i++ {
		//Gather per min, set collect data interval per 5s,however collect data interval is not an exact number eg: 5s, 6s, 8s
		//So get 1 min/ 5 sec = 20 samples. then verify the Timestamp is within latest 1 minute,then compute and gather.
		if middlestats[i].Timestamp.Add(*collectHousekeepingInterval).Add(2 * *HousekeepingInterval).Before(start) {
			continue
		} else {
			gather_stat := CumputeToGather(middlestats[i], middlestats[i+1])
			gatherstats = append(gatherstats, gather_stat)
		}
	}
	sample_num := int(*collectHousekeepingInterval) / int(*HousekeepingInterval)
	if len(gatherstats) > sample_num {
		gatherstats = gatherstats[len(gatherstats)-sample_num : len(gatherstats)]
	}
	return gatherstats
}

// Compute and transfer to gather format
func CumputeToGather(pre_stat paas.MiddleStats, now_stat paas.MiddleStats) paas.GatherStats {
	//nanosecond
	timestamp_interval := now_stat.Timestamp.UnixNano() - pre_stat.Timestamp.UnixNano()
	cpupercent := int64(now_stat.CpuTotal-pre_stat.CpuTotal) * 100 * 1000 / timestamp_interval
	fmt.Println("cpu:", now_stat.CpuTotal, pre_stat.CpuTotal, timestamp_interval, int64(now_stat.CpuTotal-pre_stat.CpuTotal)*100*1000/timestamp_interval)
	//If mem or swap does not set limit, then the limit value will be 9223372036854775807 which is definetely big.
	//When this case occurs, then set the value of mempercent or swappercent -1
	var nolimit uint64
	nolimit = 9223372036854775807
	var mempercent int64
	mempercent = -1
	if now_stat.MemLimit >= nolimit || now_stat.MemLimit == 0 {
		mempercent = -1
	} else {
		mempercent = int64(now_stat.MemUsed * 100 * 1000 / now_stat.MemLimit)
	}
	var swappercent int64
	swappercent = -1
	if now_stat.SwapLimit >= nolimit || now_stat.SwapLimit == 0 {
		swappercent = -1
	} else {
		swappercent = int64(now_stat.SwapUsed * 100 * 1000 / now_stat.SwapLimit)
	}

	ipReceivedPerSec := (now_stat.IpReceived - pre_stat.IpReceived) * 1000000000 / timestamp_interval
	ipDiscardedPerSec := (now_stat.IpDiscarded - pre_stat.IpDiscarded) * 1000000000 / timestamp_interval
	tcpReceivedPerSec := (now_stat.TcpReceived - pre_stat.TcpReceived) * 1000000000 / timestamp_interval
	tcpSendoutPerSec := (now_stat.TcpSendout - pre_stat.TcpSendout) * 1000000000 / timestamp_interval
	tcpActiveOpenPerSec := (now_stat.TcpActiveOpen - pre_stat.TcpActiveOpen) * 1000000000 / timestamp_interval
	tcpBadSegmentsPerSec := (now_stat.TcpBadSegments - pre_stat.TcpBadSegments) * 1000000000 / timestamp_interval

	//network := make(map[string]*paas.GatherInterfaceStats)
	network := make(map[string]paas.GatherInterfaceStats)
	for name, netinterface_now := range now_stat.Network {
		netinterface_pre := pre_stat.Network[name]
		rx_kb_persec := (int64(netinterface_now.RxBytes-netinterface_pre.RxBytes) / 1000) / (timestamp_interval / 1000000000)
		rx_packets := int64(netinterface_now.RxPackets - netinterface_pre.RxPackets)
		//rx_packets_persec := int64(netinterface_now.RxPackets-netinterface_pre.RxPackets) / (timestamp_interval / 1000000000)
		rx_errors_persec := int64(netinterface_now.RxErrors-netinterface_pre.RxErrors) / (timestamp_interval / 1000000000)
		tx_kb_persec := (int64(netinterface_now.TxBytes-netinterface_pre.TxBytes) / 1000) / (timestamp_interval / 1000000000)
		tx_packets := int64(netinterface_now.TxPackets - netinterface_pre.TxPackets)
		//tx_packets_persec := int64(netinterface_now.TxPackets-netinterface_pre.TxPackets) / (timestamp_interval / 1000000000)
		tx_errors_persec := int64(netinterface_now.TxErrors-netinterface_pre.TxErrors) / (timestamp_interval / 1000000000)

		//gatherInterfaceStats := &paas.GatherInterfaceStats{
		gatherInterfaceStats := paas.GatherInterfaceStats{
			RxKBPerSec: int32(rx_kb_persec),
			RxPackets:  int32(rx_packets),
			//RxPacketsPerSec: int32(rx_packets_persec),
			RxErrorsPerSec: int32(rx_errors_persec),
			TxKBPerSec:     int32(tx_kb_persec),
			//TxPacketsPerSec: int32(tx_packets_persec),
			TxPackets:      int32(tx_packets),
			TxErrorsPerSec: int32(tx_errors_persec),
		}
		network[name] = gatherInterfaceStats
	}

	//filesystem := make(map[string]*paas.GatherFsStats)
	filesystem := make(map[string]paas.GatherFsStats)
	for device, fs_now := range now_stat.FileSystem {
		fs_pre := pre_stat.FileSystem[device]
		// if not set disk limit( fs_now.Limit = 0), then usage_percent = -1
		var usage_percent int64
		usage_percent = -1
		if fs_now.Limit <= 0 {
			usage_percent = -1
		} else {
			usage_percent = int64(fs_now.Usage * 1000 / fs_now.Limit)
		}
		read_count := int64(fs_now.ReadsCompleted - fs_pre.ReadsCompleted) // / (int64(fs_now.ReadTime-fs_pre.ReadTime) / 1000)
		write_count := int64(fs_now.WritesCompleted - fs_pre.WritesCompleted)
		// 512bytes per sector default
		var sector_size int64
		sector_size = 512
		read_bytes_persec := int64(fs_now.SectorsRead-fs_pre.SectorsRead) * sector_size / (timestamp_interval / 1000000000)
		write_bytes_persec := int64(fs_now.SectorsWritten-fs_pre.SectorsWritten) * sector_size / (timestamp_interval / 1000000000)
		io_in_progress := fs_now.IoInProgress

		//gatherFsStats := &paas.GatherFsStats{
		gatherFsStats := paas.GatherFsStats{
			UsagePercent:     int32(usage_percent),
			ReadCount:        int32(read_count),
			WriteCount:       int32(write_count),
			ReadBytesPerSec:  int32(read_bytes_persec),
			WriteBytesPerSec: int32(write_bytes_persec),
			IoInProgress:     int32(io_in_progress),
		}
		filesystem[device] = gatherFsStats
	}

	gather_stat := paas.GatherStats{
		Name:                 now_stat.Name,
		IP:                   now_stat.IP,
		Timestamp:            now_stat.Timestamp.UnixNano(),
		CpuPercent:           int32(cpupercent),
		LoadAverage:          now_stat.LoadAverage,
		MemPercent:           int32(mempercent),
		SwapPercent:          int32(swappercent),
		TcpEstablished:       now_stat.TcpEstablished,
		IpReceivedPerSec:     ipReceivedPerSec,
		IpDiscardedPerSec:    ipDiscardedPerSec,
		TcpReceivedPerSec:    tcpReceivedPerSec,
		TcpSendoutPerSec:     tcpSendoutPerSec,
		TcpActiveOpenPerSec:  tcpActiveOpenPerSec,
		TcpBadSegmentsPerSec: tcpBadSegmentsPerSec,
		Network:              network,
		FileSystem:           filesystem,
	}
	//	fmt.Println("gather_stat", gather_stat)
	return gather_stat
}

func (m *manager) LocalFileHousekeeping(containerManager Manager) ([]byte, error) {
	//parse data from local file housekeeping is 5 minnute.
	//localFileHousekeeping := 5 * time.Minute
	//localFileHousekeeping := 5 * time.Second
	glog.Infof("Start parse data from local file \n")
	fmt.Println("Start parse data from local file \n")
	localFileHousekeepingInterval = LocalFileHousekeepingIntervalCheck(localFileHousekeepingInterval, collectHousekeepingInterval)
	localfile = LocalFileDirCheck(localfile)
	lastLocalFileHousekeeping := time.Now()
	for {
		select {
		//		case <-c.stop:
		// Stop housekeeping when signaled.
		//			return
		default:
			// Perform housekeeping.
			start := time.Now()
			gatherstats_all, err := paas.ParseFromLocalFile(*localfile)
			if err != nil {
				glog.Infof("parse data from local file Error:", err)
				fmt.Println("parse data from local file Error:", err)
			} else {
				//fmt.Println("parse data from local file: ", gatherstats_all)
				msg, err2 := paas.GatherData(gatherstats_all)
				if err2 != nil {
					fmt.Println("gather data Error: ", err2)
				} else {
					os.Remove(*localfile)
					fmt.Println("gather data from local file successfully", msg)
				}
			}
			// Log if housekeeping took too long.
			duration := time.Since(start)
			//fmt.Println("duration", duration)
			if duration >= *localFileHousekeepingInterval {
				glog.Infof("parse data from local file Housekeeping took %s", duration)
			}
		}

		next := lastLocalFileHousekeeping.Add(*localFileHousekeepingInterval)
		if time.Now().Before(next) {
			time.Sleep(next.Sub(time.Now()))
		} else {
			next = time.Now()
		}
		lastLocalFileHousekeeping = next
	}
}

//paas end
