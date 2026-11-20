/*
 * Copyright (c) 2026  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	nwcat "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-nw-simu-catalog"
)

func (s *SandboxCtrl) usesNem() bool {
	return s != nil && s.nwCatalog != nil && s.nwCatalog.Cfg().UsesNem()
}

func (s *SandboxCtrl) scenarioAlreadyActive() bool {
	if s == nil {
		return false
	}
	if s.nemInstance != nil {
		return true
	}
	return s.activeModel != nil && s.activeModel.Active
}

func (s *SandboxCtrl) createNemInstance(scenarioName string) error {
	if !s.usesNem() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.nwCatalog.Cfg().ActivateTimeout())
	defer cancel()

	req := nwcat.CreateInstanceRequest{
		SandboxID:    s.sandboxName,
		ScenarioID:   scenarioName,
		SimTimeLimit: nwcat.DefaultSimTimeLimit,
	}
	inst, err := s.nwCatalog.CreateInstance(ctx, req)
	if err != nil && errors.Is(err, nwcat.ErrNemConflict) {
		log.Warn("NEM instance already active for sandbox ", s.sandboxName, "; fetching status")
		inst, err = s.nwCatalog.GetInstance(ctx, s.sandboxName)
	}
	if err == nil && !nwcat.InstanceReady(inst) {
		state := ""
		if inst != nil {
			state = inst.State
		}
		err = fmt.Errorf("%w: HTTP 500: instance %s state %s", nwcat.ErrNemUnavailable, s.sandboxName, state)
	}
	if err != nil {
		s.deleteNemInstance()
		return err
	}

	s.mutex.Lock()
	s.nemInstance = inst
	s.mutex.Unlock()

	log.Info("NEM instance READY sandbox_id=", inst.SandboxID,
		" scenario_id=", inst.ScenarioID,
		" state=", inst.State,
		" slot=", inst.Slot,
		" bridge=", inst.Bridge.Target,
		" sim_host=", inst.Endpoints.SimHost,
		" traci_port=", inst.Endpoints.TraciPort,
		" rni_udp_port=", inst.Endpoints.RniUDPPort,
		" ue_telemetry_port=", inst.Endpoints.UeTelemetryPort)
	return nil
}

func (s *SandboxCtrl) deleteNemInstance() {
	if !s.usesNem() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.nwCatalog.Cfg().ActivateTimeout())
	defer cancel()

	err := s.nwCatalog.DeleteInstance(ctx, s.sandboxName, false)
	if err != nil {
		log.Error("Failed to delete NEM instance ", s.sandboxName, ": ", err.Error())
	} else {
		log.Info("NEM instance deleted sandbox_id=", s.sandboxName)
	}
	s.mutex.Lock()
	s.nemInstance = nil
	s.mutex.Unlock()
}

func (s *SandboxCtrl) writeNemActiveScenario(w http.ResponseWriter) bool {
	s.mutex.Lock()
	inst := s.nemInstance
	s.mutex.Unlock()
	if inst == nil {
		return false
	}
	name := inst.ScenarioID
	if name == "" {
		name = s.sandboxName
	}
	body, err := json.Marshal(struct {
		Name       string                 `json:"name"`
		Deployment map[string]interface{} `json:"deployment"`
	}{
		Name:       name,
		Deployment: map[string]interface{}{"domains": []interface{}{}},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, string(body))
	return true
}

func mapNemActivateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, nwcat.ErrNemConflict):
		http.Error(w, nwcat.UserMessage(err), http.StatusConflict)
	case errors.Is(err, nwcat.ErrNemBadRequest):
		http.Error(w, nwcat.UserMessage(err), http.StatusBadRequest)
	default:
		log.Error("NEM instance create: ", err.Error())
		if nwcat.Unreachable(err) {
			http.Error(w, nwcat.ErrMsgNemNotRunning, http.StatusBadGateway)
			return
		}
		http.Error(w, nwcat.UserMessage(err), http.StatusInternalServerError)
	}
}
