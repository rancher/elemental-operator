package prober

import (
	"sync"

	"github.com/sirupsen/logrus"
)

func DoProbes(probes map[string]Probe, probeStatuses map[string]ProbeStatus, initial bool) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for probeName, probe := range probes {
		wg.Add(1)
		go func(probeName string, probe Probe, wg *sync.WaitGroup) {
			defer wg.Done()
			logrus.Debugf("[Prober] (%s) running probe", probeName)
			mu.Lock()
			logrus.Tracef("[Prober] (%s) retrieving existing probe status from map if existing", probeName)
			probeStatus, ok := probeStatuses[probeName]
			mu.Unlock()
			if !ok {
				logrus.Tracef("[Prober] (%s) probe status was not present in map, initializing", probeName)
				probeStatus = ProbeStatus{}
			}
			probe.Name = probeName
			if err := DoProbe(probe, &probeStatus, initial); err != nil {
				logrus.Errorf("error running probe %s", probeName)
			}
			mu.Lock()
			logrus.Tracef("[Prober] (%s) writing probe status to map", probeName)
			probeStatuses[probeName] = probeStatus
			mu.Unlock()
		}(probeName, probe, &wg)
	}
	// wait for all probes to complete
	wg.Wait()
}
