package src

import (
	"context"
	"encoding/json"
	"google.golang.org/api/compute/v1"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var project string = os.Getenv("GCP_PROJECT")

type Message struct {
	Data []byte `json:"data"`
}

type Payload struct {
	LabelKey   string `json:"labelKey"`
	LabelValue string `json:"labelValue"`
}

type InstanceService struct {
	service        *compute.Service
	aggregatedList *compute.InstancesAggregatedListCall
	project        string
	err            error
}

type Result struct {
	NumOfTargetInstances        int
	StoppedInstanceNames        []string
	AlreadyStoppedInstanceNames []string
}

func init() {
	log.Printf("GCP_PROJECT: %v", project)
}

func StopGCEInstances(ctx context.Context, msg Message) error {
	var payload Payload
	err := json.Unmarshal(msg.Data, &payload)
	if err != nil {
		return err
	}

	result, err := GCE(ctx).Filter(payload.LabelKey, payload.LabelValue).Stop()
	if err != nil {
		return err
	} else {
		log.Println(result.Show())
	}

	return err
}

func GCE(ctx context.Context) *InstanceService {
	service, _ := compute.NewService(ctx)

	return &InstanceService{
		service:        service,
		aggregatedList: compute.NewInstancesService(service).AggregatedList(project),
		project:        project,
		err:            nil,
	}
}

func (instanceService *InstanceService) Filter(labelKeyName string, labelValueName string) *InstanceService {
	if instanceService.err != nil {
		return instanceService
	}

	filter := "labels." + labelKeyName + "=" + labelValueName
	instanceService.aggregatedList = instanceService.aggregatedList.Filter(filter)

	return instanceService
}

func (instanceService *InstanceService) Stop() (*Result, error) {
	if instanceService.err != nil {
		return nil, instanceService.err
	}

	instanceAggregatedList, err := instanceService.aggregatedList.Do()
	if err != nil {
		return nil, err
	}

	// TODO:並列化
	var stopped_names []string
	var already_stopped_names []string
	for _, instance := range Instances(instanceAggregatedList.Items) {
		status := instance.Status
		if status == "STOPPED" || status == "STOPPING" || status == "TERMINATED" ||
			status == "PROVISIONING" || status == "REPAIRING" {
			already_stopped_names = append(already_stopped_names, instance.Name)
			continue
		}

		zoneURL := strings.Split(instance.Zone, "/")
		zone := zoneURL[len(zoneURL)-1]

		_, err = compute.NewInstancesService(instanceService.service).Stop(instanceService.project, zone, instance.Name).Do()
		if err != nil {
			return nil, err
		}

		stopped_names = append(stopped_names, instance.Name)
		time.Sleep(50 * time.Millisecond)
	}

	return &Result{
		NumOfTargetInstances:        len(stopped_names) + len(already_stopped_names),
		StoppedInstanceNames:        stopped_names,
		AlreadyStoppedInstanceNames: already_stopped_names,
	}, nil
}

func Instances(scopedList map[string]compute.InstancesScopedList) (instances []*compute.Instance) {
	for _, instanceList := range scopedList {
		if len(instanceList.Instances) == 0 {
			continue
		}
		instances = append(instances, instanceList.Instances...)
	}
	return
}

func (result *Result) Show() (line string) {
	f := func(content string, nameList []string) {
		line += "\n" + content + ": " + strconv.Itoa(len(nameList))
		line += "("
		for i, name := range nameList {
			if i == 0 {
				line += name
			} else {
				line += name + ", "
			}
		}
		line += ")\n"
	}
	f("Stopped", result.StoppedInstanceNames)
	f("Already stopped", result.AlreadyStoppedInstanceNames)

	return
}
