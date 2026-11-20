/*
 * Copyright (c) 2022  The AdvantEDGE Authors
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

/*
*
This package implements MEC-016 application instantiation with the following limitation:
1) The application image is already available in the MEC system (AdvantEDGE platform)
2) Onboarding application is not supported (MEC-010-2)
3) Application list is hard-coded (to be enhance in the furture)
*/
package meepdaimgr

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strconv"

	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"

	uuid "github.com/google/uuid"
	memdb "github.com/hashicorp/go-memdb"
)

// Enable profiling
const profiling = false

var profilingTimers map[string]time.Time

type Uri string

// ETSI GS MEC 016 Clause Table 6.2.2-1: Definition of type ApplicationList
type ApplicationList struct {
	AppList           []AppInfo
	VendorSpecificExt *VendorSpecificExt
}

type VendorSpecificExt struct {
	VendorId string
}

// ETSI GS MEC 016 Clause Table 6.2.2-1: Definition of type ApplicationList
type AppInfo struct {
	AppDId         string
	AppName        string
	AppProvider    string
	AppSoftVersion string
	AppDVersion    string
	AppDescription string
	AppLocation    *[]LocationConstraintsItem
	AppCharcs      *AppCharcs
}

type AppInfoTable struct {
	id             int
	AppDId         string
	AppName        string
	AppProvider    string
	AppSoftVersion string
	AppDVersion    string
	AppDescription string
}

// ETSI GS MEC 016 Clause Table 6.2.2-1: Definition of type ApplicationList
type AppCharcs struct {
	Memory      *uint32
	Storage     *uint32
	Latency     *uint32
	Bandwidth   *uint32
	ServiceCont *uint32
}

type AppCharcsTable struct {
	id          int
	AppDId      string
	Memory      int
	Storage     int
	Latency     int
	Bandwidth   int
	ServiceCont int
}

// ETSI GS MEC 016 Clause 6.2.3 Type: AppContext
type AppContext struct {
	ContextId            string // Uniquely identifies the application context in the MEC system
	AssociateDevAppId    string // Uniquely identifies the device application
	CallbackReference    *Uri
	AppLocationUpdates   bool
	AppAutoInstantiation bool
	AppInfo              AppInfoContext
}

type AppContextTable struct {
	id                   int
	ContextId            string // Uniquely identifies the application context in the MEC system
	AssociateDevAppId    string // Uniquely identifies the device application
	CallbackReference    Uri
	AppLocationUpdates   bool
	AppAutoInstantiation bool
	AppDId               string
}

// ETSI GS MEC 016 Clause 6.2.3 Type: AppContext
type AppInfoContext struct {
	AppDId              string
	AppName             string
	AppProvider         string
	AppSoftVersion      *string
	AppDVersion         string
	AppDescription      *string
	UserAppInstanceInfo UserAppInstanceInfo
}

type AppInfoContextTable struct {
	id             int
	ContextId      string // Uniquely identifies the application context in the MEC system
	AppDId         string
	AppName        string
	AppProvider    string
	AppSoftVersion string
	AppDVersion    string
	AppDescription string
}

// ETSI GS MEC 016 Clause 6.2.3 Type: AppContext
type UserAppInstanceInfo []UserAppInstanceInfoItem
type UserAppInstanceInfoItem struct {
	AppInstanceId string
	ReferenceURI  Uri
	AppLocation   *LocationConstraintsItem
}

type AppLocationTable struct {
	id                  int
	AppInstanceId       string
	Area                string
	CivicAddressElement string
	CountryCode         string
}

type UserAppInstanceInfoItemTable struct {
	id            int
	AppDId        string
	AppInstanceId string
	ReferenceURI  Uri
}

// ETSI GS MEC 016 Clause 6.5.2 Type: LocationConstraints
type LocationConstraints []LocationConstraintsItem
type LocationConstraintsItem struct {
	Area                *Polygon
	CivicAddressElement *CivicAddressElement
	CountryCode         *string
}

type LocationConstraintsTable struct {
	id                  int
	AppDId              string
	Area                string
	CivicAddressElement string
	CountryCode         string
}

type Polygon struct {
	Coordinates [][][]float32 `json:"coordinates"`
}
type CivicAddressElement []CivicAddressElementItem
type CivicAddressElementItem struct {
	CaType  int32  `json:"caType,omitempty"`
	CaValue string `json:"caValue,omitempty"`
}

// ETSI GS MEC 016 Clause 6.2.4 Type: ApplicationLocationAvailability
type ApplicationLocationAvailability struct {
	AssociateDevAppId string
	AppInfo           ApplicationLocationAvailabilityAppInfo
}

// ETSI GS MEC 016 Clause 6.2.3 Type: AppContext
type ApplicationLocationAvailabilityAppInfo struct {
	AppName            string
	AppProvider        string
	AppSoftVersion     *string
	AppDVersion        string
	AppDescription     string
	AvailableLocations *[]AvailableLocationsItem
	AppPackageSource   *Uri
}

type ApplicationLocationAvailabilityAppInfoTable struct {
	id               int
	AppName          string
	AppProvider      string
	AppSoftVersion   string
	AppDVersion      string
	AppDescription   string
	AppPackageSource Uri
}

// ETSI GS MEC 016 Clause 6.2.4 Type: ApplicationLocationAvailability
type AvailableLocations []AvailableLocationsItem

// ETSI GS MEC 016 Clause 6.2.4 Type: ApplicationLocationAvailability
type AvailableLocationsItem struct {
	AppLocation *LocationConstraintsItem
}

type AppExecEntry struct {
	cmd    *exec.Cmd
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

var appExecEntries map[int]AppExecEntry = make(map[int]AppExecEntry)

// DAI configuration
type DaiCfg struct {
	NotifyAppContextDeletion func(string, string)
}

// DAI Manager
type DaiMgr struct {
	db            *memdb.MemDB
	schema        *memdb.DBSchema
	tickerStarted bool
	updateCb      func(string)
}

var daiMgr *DaiMgr
var appContexts map[string]AppContext = make(map[string]AppContext)
var notifyAppContextDeletion func(string, string)
var processCheckExpiry time.Duration = 5 * time.Second
var processCheckTicker *time.Ticker

var dbId int = 0

// Profiling init
func init() {
	if profiling {
		profilingTimers = make(map[string]time.Time)
	}
}

// NewDaiMgr - Creates and initializes a new DAI Manager
func NewDaiMgr(cfg DaiCfg) (am *DaiMgr, err error) {
	log.Debug(">>> NewDaiMgr: ", cfg)

	// Create new Asset Manager
	am = new(DaiMgr)

	err = am.createDb()
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	// Start process checking timer
	am.tickerStarted = cfg.NotifyAppContextDeletion != nil
	if cfg.NotifyAppContextDeletion != nil {
		notifyAppContextDeletion = cfg.NotifyAppContextDeletion
		startProcessCheckTicker()
		log.Info("ProcessCheckTicker successfully started")
	}

	daiMgr = am

	return am, nil
}

func (am *DaiMgr) SetListener(listener func(string)) error {
	am.updateCb = listener
	return nil
}

func (am *DaiMgr) notifyListener(assetName string) {
	if am.updateCb != nil {
		go am.updateCb(assetName)
	}
}

// DeleteDaiMgr -
func (am *DaiMgr) DeleteDaiMgr() (err error) {
	am.schema = nil
	am.db = nil
	return nil
}

// createDb -- Create new DB with provided name
func (am *DaiMgr) createDb() (err error) {
	log.Debug(">>> createDb")

	// Sanity checks
	if am.db != nil {
		err = errors.New("DB already exist, cannot create a new one")
		log.Error(err.Error())
		return err
	}

	err = am.createTables()
	if err != nil {
		log.Error(err.Error())
		am.schema = nil
		return err
	}

	// Create a new data base
	am.db, err = memdb.NewMemDB(am.schema)
	if err != nil {
		log.Error(err.Error())
		am.schema = nil
		return err
	}

	log.Info("Created database")
	return nil
}

func (am *DaiMgr) createTables() (err error) {
	log.Debug(">>> createTables")

	am.schema = &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			"AppInfoTable": &memdb.TableSchema{ // AppInfoList Table
				// TODO meep-dai-mgr could be partially enhanced with MEC-10-2 Clauses 6.2.1.2 Type: AppD and Lifecycle Mgmt
				Name: "AppInfoTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"AppDId": &memdb.IndexSchema{
						Name:    "AppDId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AppDId"},
					},
					"AppName": &memdb.IndexSchema{
						Name:    "AppName",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppName"},
					},
					"AppProvider": &memdb.IndexSchema{
						Name:    "AppProvider",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppProvider"},
					},
					"AppSoftVersion": &memdb.IndexSchema{
						Name:    "AppSoftVersion",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppSoftVersion"},
					},
					"AppDVersion": &memdb.IndexSchema{
						Name:    "AppDVersion",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppDVersion"},
					},
					"AppDescription": &memdb.IndexSchema{
						Name:    "AppDescription",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppDescription"},
					},
				},
			},
			"LocationConstraintsTable": &memdb.TableSchema{ // UserAppInstanceInfo Table
				// Warning: appInstanceId shall be present
				Name: "LocationConstraintsTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"AppDId": &memdb.IndexSchema{
						Name:    "AppDId",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppDId"},
					},
					"Area": &memdb.IndexSchema{
						Name:    "Area",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "Area"},
					},
					"CivicAddressElement": &memdb.IndexSchema{
						Name:    "CivicAddressElement",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "CivicAddressElement"},
					},
					"CountryCode": &memdb.IndexSchema{
						Name:    "CountryCode",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "CountryCode"},
					},
				},
			},
			"AppCharcsTable": &memdb.TableSchema{ // AppCharcs Table
				Name: "AppCharcsTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"AppDId": &memdb.IndexSchema{
						Name:    "AppDId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AppDId"},
					},
					"Memory": &memdb.IndexSchema{
						Name:    "Memory",
						Unique:  false,
						Indexer: &memdb.IntFieldIndex{Field: "Memory"},
					},
					"Storage": &memdb.IndexSchema{
						Name:    "Storage",
						Unique:  false,
						Indexer: &memdb.IntFieldIndex{Field: "Storage"},
					},
					"Latency": &memdb.IndexSchema{
						Name:    "Latency",
						Unique:  false,
						Indexer: &memdb.IntFieldIndex{Field: "Latency"},
					},
					"Bandwidth": &memdb.IndexSchema{
						Name:    "Bandwidth",
						Unique:  false,
						Indexer: &memdb.IntFieldIndex{Field: "Bandwidth"},
					},
					"ServiceCont": &memdb.IndexSchema{
						Name:    "ServiceCont",
						Unique:  false,
						Indexer: &memdb.IntFieldIndex{Field: "ServiceCont"},
					},
				},
			},
			"UserAppInstanceInfoItemTable": &memdb.TableSchema{ // UserAppInstanceInfo
				// Warning: appInstanceId shall be present
				Name: "UserAppInstanceInfoItemTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"AppDId": &memdb.IndexSchema{
						Name:    "AppDId",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppDId"},
					},
					"AppInstanceId": &memdb.IndexSchema{
						Name:    "AppInstanceId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AppInstanceId"},
					},
					"ReferenceURI": &memdb.IndexSchema{
						Name:    "ReferenceURI",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "ReferenceURI"},
					},
				},
			},
			"AppLocationTable": &memdb.TableSchema{ // UserAppInstanceInfo
				// Warning: appInstanceId shall be present
				Name: "AppLocationTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"AppInstanceId": &memdb.IndexSchema{
						Name:    "AppInstanceId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AppInstanceId"},
					},
					"Area": &memdb.IndexSchema{
						Name:    "Area",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "Area"},
					},
					"CivicAddressElement": &memdb.IndexSchema{
						Name:    "CivicAddressElement",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "CivicAddressElement"},
					},
					"CountryCode": &memdb.IndexSchema{
						Name:    "CountryCode",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "CountryCode"},
					},
				},
			},
			"AppContextTable": &memdb.TableSchema{ // AppContext Table
				// TODO meep-dai-mgr could be partially enhanced with MEC-10-2 Clauses 6.2.1.2 Type: AppD and Lifecycle Mgmt
				Name: "AppContextTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"ContextId": &memdb.IndexSchema{
						Name:    "ContextId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "ContextId"},
					},
					"AssociateDevAppId": &memdb.IndexSchema{
						Name:    "AssociateDevAppId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AssociateDevAppId"},
					},
					"CallbackReference": &memdb.IndexSchema{
						Name:    "CallbackReference",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "CallbackReference"},
					},
					"AppLocationUpdates": &memdb.IndexSchema{
						Name:    "AppLocationUpdates",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppLocationUpdates"},
					},
					"AppAutoInstantiation": &memdb.IndexSchema{
						Name:    "AppAutoInstantiation",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppAutoInstantiation"},
					},
					"AppDId": &memdb.IndexSchema{
						Name:    "AppDId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AppDId"},
					},
				},
			},
			"AppInfoContextTable": &memdb.TableSchema{ // AppInfo Table
				// TODO meep-dai-mgr could be partially enhanced with MEC-10-2 Clauses 6.2.1.2 Type: AppD and Lifecycle Mgmt
				Name: "AppInfoContextTable",
				Indexes: map[string]*memdb.IndexSchema{
					"id": &memdb.IndexSchema{
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "id"},
					},
					"ContextId": &memdb.IndexSchema{
						Name:    "ContextId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "ContextId"},
					},
					"AppDId": &memdb.IndexSchema{
						Name:    "AppDId",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "AppDId"},
					},
					"AppName": &memdb.IndexSchema{
						Name:    "AppName",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppName"},
					},
					"AppProvider": &memdb.IndexSchema{
						Name:    "AppProvider",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppProvider"},
					},
					"AppSoftVersion": &memdb.IndexSchema{
						Name:    "AppSoftVersion",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppSoftVersion"},
					},
					"AppDVersion": &memdb.IndexSchema{
						Name:    "AppDVersion",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppDVersion"},
					},
					"AppDescription": &memdb.IndexSchema{
						Name:    "AppDescription",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "AppDescription"},
					},
				},
			},
		},
	}

	err = am.schema.Validate()
	if err != nil {
		am.schema = nil
		log.Error(err.Error())
		return err
	}

	return nil
}

// deleteTables - Delete all postgis tables
func (am *DaiMgr) deleteTables() (err error) {
	_ = am.deleteTable("AppContextTable")
	_ = am.deleteTable("AppInfoContextTable")
	_ = am.deleteTable("UserAppInstanceInfoTable")
	_ = am.deleteTable("LocationConstraintsTable")
	_ = am.deleteTable("AvailableLocationsTable")
	_ = am.deleteTable("ApplicationLocationAvailabilityTable")
	_ = am.deleteTable("AppInfoTable")
	_ = am.deleteTable("AppCharcsTable")
	return nil
}

// deleteTable - Delete postgis table with provided name
func (am *DaiMgr) deleteTable(tableName string) (err error) {
	txn := am.db.Txn(true)
	err = txn.Delete(tableName, "AppDId")
	if err != nil {
		log.Error(err.Error())
		return err
	}
	txn.Commit()

	return nil
}

// DeleteAppInfoRecord - Delete DeleteAppContextTable entry
func (am *DaiMgr) DeleteAppInfoRecord(appDId string) (err error) {
	if profiling {
		profilingTimers["DeleteAppInfoRecord"] = time.Now()
	}

	// Validate input
	if appDId == "" {
		err = errors.New("Missing appDId")
		return err
	}

	txn := am.db.Txn(true)
	err = txn.Delete("AppInfoTable", AppInfoTable{AppDId: appDId})
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("LocationConstraintsTable", "AppDId", appDId)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("AppCharcsTable", "id")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	txn.Commit()

	if profiling {
		now := time.Now()
		log.Debug("DeleteAppInfoRecord: ", now.Sub(profilingTimers["DeleteAppInfoRecord"]))
	}

	return nil
}

// DeleteAppContext - Delete all DeleteAppContextTable entries
func (am *DaiMgr) DeleteAllAppInfoRecord() (err error) {
	if profiling {
		profilingTimers["DeleteAllAppInfoRecord"] = time.Now()
	}

	txn := am.db.Txn(true)
	_, err = txn.DeleteAll("AppInfoTable", "id")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("LocationConstraintsTable", "id")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("AppCharcsTable", "id")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	txn.Commit()

	if profiling {
		now := time.Now()
		log.Debug("DeleteAllAppInfoRecord: ", now.Sub(profilingTimers["DeleteAllAppInfoRecord"]))
	}

	return nil
}

// DeleteAppContext - Delete DeleteAppContextTable entry
func (am *DaiMgr) DeleteAppContext(appContextId string) (err error) {
	if profiling {
		profilingTimers["DeleteAppContext"] = time.Now()
	}

	// Validate input
	if appContextId == "" {
		err = errors.New("Missing appContextId")
		return err
	}

	// // Whe deleting the context, un-instantiate the application
	// // TODO meep-dai-mgr could be partially enhanced with MEC-10-2 Clauses 6.2.1.2 Type: AppD and Lifecycle Mgmt
	// // Un-instantiate the MEC application process
	// pid, err := strconv.ParseInt(appContextId, 10, 64) // FIXME To be enhanced to get outputs
	// if err != nil {
	// 	log.Error(err.Error())
	// 	return err
	// }
	// // TODO Check if the process is running

	// terminatePidProcess(int(pid))
	// log.Debug("Just terminated subprocess ", strconv.Itoa(int(pid)))
	// // Delete entries
	// delete(appExecEntries, int(pid))
	// delete(appContexts, strconv.Itoa(int(pid)))

	txn := am.db.Txn(false)
	raw, err := txn.First("AppContextTable", "ContextId", appContextId)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	txn.Abort()
	if raw == nil {
		err = errors.New("Wrong ContextId")
		log.Error(err.Error())
		return err
	}

	txn = am.db.Txn(true)
	err = txn.Delete("AppContextTable", AppContextTable{id: raw.(*AppContextTable).id}) // BUG Unable to delete record based on ContextId
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("AppInfoContextTable", "ContextId", appContextId)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("UserAppInstanceInfoItemTable", "AppDId", raw.(*AppContextTable).AppDId)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	txn.Commit()

	// Notify listener
	am.notifyListener(appContextId)

	if profiling {
		now := time.Now()
		log.Debug("DeleteAppContext: ", now.Sub(profilingTimers["DeleteAppContext"]))
	}

	return nil
}

// DeleteAppContext - Delete all DeleteAppContextTable enterprises
// Only for testing purpose
func (am *DaiMgr) DeleteAllAppContext() (err error) {
	if profiling {
		profilingTimers["DeleteAllAppContext"] = time.Now()
	}

	txn := am.db.Txn(true)
	_, err = txn.DeleteAll("AppContextTable", "ContextId")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("AppInfoContextTable", "ContextId")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	_, err = txn.DeleteAll("UserAppInstanceInfoItemTable", "AppDId")
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	txn.Commit()

	if profiling {
		now := time.Now()
		log.Debug("DeleteAllAppContext: ", now.Sub(profilingTimers["DeleteAllAppContext"]))
	}

	return nil
}

// LoadOnboardedMecApplications -- This function fetches the dynamic app catalog from meep-virt-engine
func (am *DaiMgr) LoadOnboardedMecApplications(folder string) (err error) {
	log.Info("LoadOnboardedMecApplications: fetching from meep-virt-engine")

	resp, err := http.Get("http://meep-virt-engine.default.svc.cluster.local/virt-engine/v1/onboard_app")
	if err != nil {
		log.Error("Failed to fetch catalog: ", err.Error())
		return err
	}
	defer resp.Body.Close()

	byteValue, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error("Failed to read response body: ", err.Error())
		return err
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("virt-engine returned status: ", resp.StatusCode)
		return errors.New("Failed to fetch catalog")
	}

	var catalog []map[string]interface{}
	if err := json.Unmarshal(byteValue, &catalog); err != nil {
		log.Error("Failed to parse catalog JSON: ", err.Error())
		return err
	}

	var onboardedAppList ApplicationList
	for _, appDMap := range catalog {
		// Convert map back to JSON string to use existing convert logic
		appDBytes, _ := json.Marshal(appDMap)
		log.Info("LoadOnboardedMecApplications: Processing appD ", string(appDBytes))

		applicationList := convertJsonToApplicationList(string(appDBytes))
		if applicationList == nil || len(applicationList.AppList) == 0 {
			log.Error("Failed to convert appD or missing AppInfoList")
			continue
		}

		for _, app := range applicationList.AppList {
			err = am.CreateAppInfoRecord(app)
			if err != nil {
				log.Error(err.Error())
				continue
			}
			onboardedAppList.AppList = append(onboardedAppList.AppList, app)
		}
	}

	if len(onboardedAppList.AppList) == 0 {
		log.Error("No onboarded MEC application found")
	} else {
		log.Info("Created onboarded user applications")
	}

	return nil
}

func (am *DaiMgr) CreateAppInfoRecord(appInfo AppInfo) (err error) {
	log.Info(">>> CreateAppInfoRecord: ", appInfo)

	if profiling {
		profilingTimers["CreateAppInfoRecord"] = time.Now()
	}

	// Sanity checks
	if appInfo.AppDId == "" { // ETSI GS MEC 016 Clause 6.2.2 Type: ApplicationList
		return errors.New("Missing AppDId")
	}
	if appInfo.AppName == "" { // ETSI GS MEC 016 Clause 6.2.2 Type: ApplicationList
		return errors.New("Missing AppName")
	}
	if appInfo.AppProvider == "" { // ETSI GS MEC 016 Clause 6.2.2 Type: ApplicationList
		return errors.New("Missing AppProvider")
	}
	if appInfo.AppSoftVersion == "" { // ETSI GS MEC 016 Clause 6.2.2 Type: ApplicationList
		return errors.New("Missing AppSoftVersion")
	}
	if appInfo.AppDVersion == "" { // ETSI GS MEC 016 Clause 6.2.2 Type: ApplicationList
		return errors.New("Missing AppDVersion")
	}
	if appInfo.AppDescription == "" { // ETSI GS MEC 016 Clause 6.2.2 Type: ApplicationList
		return errors.New("Missing AppDescription")
	}

	// Create AppInfo entry
	txn := am.db.Txn(true)
	newDbId := dbId
	record := &AppInfoTable{newDbId, appInfo.AppDId, appInfo.AppName, appInfo.AppProvider, appInfo.AppSoftVersion, appInfo.AppDVersion, appInfo.AppDescription}
	err = txn.Insert("AppInfoTable", record)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return err
	}
	newDbId += 1

	// Create AppInfo.AppLocation entries if any
	if appInfo.AppLocation != nil && len(*appInfo.AppLocation) != 0 {
		for _, appLocation := range *appInfo.AppLocation {
			record := &LocationConstraintsTable{newDbId, appInfo.AppDId, convertPolygonToJson(appLocation.Area), convertCivicAddressElementToJson(appLocation.CivicAddressElement), NilToEmptyString(appLocation.CountryCode)}
			err = txn.Insert("LocationConstraintsTable", record)
			if err != nil {
				txn.Abort()
				log.Error(err.Error())
				return err
			}
			newDbId += 1
		} // End of 'for' statement
	}
	// Create AppInfo.AppCharcs entries if any
	if appInfo.AppCharcs != nil {
		record := &AppCharcsTable{newDbId, appInfo.AppDId, int(NilToEmptyUInt32(appInfo.AppCharcs.Memory)), int(NilToEmptyUInt32(appInfo.AppCharcs.Storage)), int(NilToEmptyUInt32(appInfo.AppCharcs.Latency)), int(NilToEmptyUInt32(appInfo.AppCharcs.Bandwidth)), int(NilToEmptyUInt32(appInfo.AppCharcs.ServiceCont))}
		err = txn.Insert("AppCharcsTable", record)
		if err != nil {
			txn.Abort()
			log.Error(err.Error())
			return err
		}
		newDbId += 1
	}
	txn.Commit()
	dbId = newDbId

	if profiling {
		now := time.Now()
		log.Debug("CreateAppInfoRecord: ", now.Sub(profilingTimers["CreateAppInfoRecord"]))
	}

	return nil
}

func (am *DaiMgr) GetAppInfoRecord(appDId string) (appInfo *AppInfo, err error) {
	if profiling {
		profilingTimers["GetAppInfoRecord"] = time.Now()
	}

	// Sanity check
	if appDId == "" {
		err = errors.New("Missing AppDId")
		log.Error(err.Error())
		return nil, err
	}

	txn := am.db.Txn(false)
	defer txn.Abort()

	raw, err := txn.First("AppInfoTable", "AppDId", appDId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if raw == nil {
		err = errors.New("Wrong AppDId")
		log.Error(err.Error())
		return nil, err
	}
	appInfo, err = am.processAppInfoRecord(txn, raw.(*AppInfoTable))
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	if profiling {
		now := time.Now()
		log.Debug("GetAppInfoRecord: ", now.Sub(profilingTimers["GetAppInfoRecord"]))
	}

	return appInfo, nil
}

func (am *DaiMgr) GetAllAppInfoRecord() (appInfoList map[string]*AppInfo, err error) {
	if profiling {
		profilingTimers["GetAllAppInfoRecord"] = time.Now()
	}

	txn := am.db.Txn(false)
	defer txn.Abort()

	it, err := txn.Get("AppInfoTable", "AppDId")
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if it != nil {
		// Create AppInfo map
		appInfoList = make(map[string]*AppInfo)

		for obj := it.Next(); obj != nil; obj = it.Next() {
			appInfo, err := am.processAppInfoRecord(txn, obj.(*AppInfoTable))
			if err != nil {
				continue // Skip it
			}
			appInfoList[appInfo.AppDId] = appInfo
		} // End of 'for' statement
	}

	if appInfoList == nil {
		err = errors.New("AppInfoList not found")
		return nil, err
	}

	if profiling {
		now := time.Now()
		log.Debug("GetAllAppInfoRecord: ", now.Sub(profilingTimers["GetAllAppInfoRecord"]))
	}

	return appInfoList, nil
}

func (am *DaiMgr) CreateAppContext(appContext *AppContext, remoteUrl string, sanboxName string) (app *AppContext, err error) {
	if profiling {
		profilingTimers["CreateAppContext"] = time.Now()
	}

	// Sanity checks
	if appContext == nil {
		return nil, errors.New("CreateAppContext: Invalid input parameters")
	}
	app = appContext

	// Whe creating the context, instantiate the application
	// Retrieve the MEC application description
	appInfo, err := am.GetAppInfoRecord(app.AppInfo.AppDId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	log.Debug("CreateAppContext: appInfo ", appInfo)

	// appExecEntry, err := cmdExec(appInfo.Cmd)
	// if err != nil {
	// 	log.Error(err.Error())
	// 	return nil, err
	// }
	// log.Debug("Just ran subprocess ", strconv.Itoa(appExecEntry.cmd.Process.Pid))
	// targetIp := getOutboundIP()
	// log.Debug("CreateAppContext: targetIp: ", targetIp)

	// Generate ContextId
	app.ContextId = uuid.New().String()[0:30] // length limited to 30 chars for scenario node names
	for i := range app.AppInfo.UserAppInstanceInfo {
		app.AppInfo.UserAppInstanceInfo[i].AppInstanceId = uuid.New().String()[0:30] // length limited to 30 chars
		app.AppInfo.UserAppInstanceInfo[i].ReferenceURI = Uri(remoteUrl + "/" + sanboxName + "/" + app.AppInfo.AppName)
		app.AppInfo.UserAppInstanceInfo[i].ReferenceURI = Uri(string(app.AppInfo.UserAppInstanceInfo[i].ReferenceURI)) //Uri(strings.Replace(string(*app.AppInfo.UserAppInstanceInfo[i].ReferenceURI), "http:", "https:", 1))
	} // End of 'for' statement
	log.Debug("CreateAppContext: app.ContextId: ", app.ContextId)
	log.Debug("CreateAppContext: app.AppInfo: ", app.AppInfo)
	log.Debug("CreateAppContext: app.AppInfo.UserAppInstanceInfo: ", app.AppInfo.UserAppInstanceInfo)

	// Create AppContext entry
	txn := am.db.Txn(true)
	newDbId := dbId
	record := &AppContextTable{newDbId, app.ContextId, app.AssociateDevAppId, NilToEmptyUri(app.CallbackReference), app.AppLocationUpdates, app.AppAutoInstantiation, app.AppInfo.AppDId}
	err = txn.Insert("AppContextTable", record)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return nil, err
	}
	newDbId += 1

	// Create AppInfo entry
	record1 := &AppInfoContextTable{newDbId, app.ContextId, app.AppInfo.AppDId, app.AppInfo.AppName, app.AppInfo.AppProvider, NilToEmptyString(app.AppInfo.AppSoftVersion), app.AppInfo.AppDVersion, NilToEmptyString(app.AppInfo.AppDescription)}
	err = txn.Insert("AppInfoContextTable", record1)
	if err != nil {
		txn.Abort()
		log.Error(err.Error())
		return nil, err
	}
	newDbId += 1

	// Create UserAppInstanceInfo entry
	for i := range app.AppInfo.UserAppInstanceInfo {
		record2 := &UserAppInstanceInfoItemTable{newDbId, app.AppInfo.AppDId, app.AppInfo.UserAppInstanceInfo[i].AppInstanceId, app.AppInfo.UserAppInstanceInfo[i].ReferenceURI}
		err = txn.Insert("UserAppInstanceInfoItemTable", record2)
		if err != nil {
			txn.Abort()
			log.Error(err.Error())
			return nil, err
		}
		newDbId += 1
		// Create AppLocation entry
		record3 := &AppLocationTable{newDbId, app.AppInfo.UserAppInstanceInfo[i].AppInstanceId, convertPolygonToJson(app.AppInfo.UserAppInstanceInfo[i].AppLocation.Area), convertCivicAddressElementToJson(app.AppInfo.UserAppInstanceInfo[i].AppLocation.CivicAddressElement), NilToEmptyString(app.AppInfo.UserAppInstanceInfo[i].AppLocation.CountryCode)}
		err = txn.Insert("AppLocationTable", record3)
		if err != nil {
			txn.Abort()
			log.Error(err.Error())
			return nil, err
		}
		newDbId += 1
	} // End of 'for' statement

	txn.Commit()
	dbId = newDbId

	// process, err := os.FindProcess(int(appExecEntry.cmd.Process.Pid))
	// if err != nil {
	// 	log.Error(err.Error())
	// }
	// log.Debug("Process info: ", *process)
	// appExecEntries[appExecEntry.cmd.Process.Pid] = appExecEntry
	// log.Debug("appExecEntries ", appExecEntries)
	// appContexts[app.ContextId] = *app
	// log.Debug("appContexts ", appContexts)

	// Notify listener
	am.notifyListener(app.ContextId)

	if profiling {
		now := time.Now()
		log.Debug("CreateAppContext: ", now.Sub(profilingTimers["CreateAppContext"]))
	}

	return app, nil
}

func (am *DaiMgr) PutAppContext(appContext AppContext) (err error) {
	if profiling {
		profilingTimers["PutAppContext"] = time.Now()
	}

	// Retrieve the existing AppContext
	curAppContext, err := am.GetAppContextRecord(appContext.ContextId)
	if err != nil {
		return errors.New("ContextId not found")
	}
	log.Debug("PutAppContext: curAppContext: ", curAppContext)

	// Update the curAppContext
	update := false
	// if curAppContext.CallbackReference != appContext.CallbackReference {
	// 	curAppContext.CallbackReference = appContext.CallbackReference
	// 	update = true
	// }

	if update {
		txn := am.db.Txn(false)
		raw, err := txn.First("AppContextTable", "ContextId", curAppContext.ContextId)
		if err != nil {
			txn.Abort()
			log.Error(err.Error())
			return err
		}
		txn.Abort()
		if raw == nil {
			err = errors.New("Wrong ContextId")
			log.Error(err.Error())
			return err
		}

		txn = am.db.Txn(true)
		record := &AppContextTable{
			ContextId:            curAppContext.ContextId,
			AssociateDevAppId:    curAppContext.AssociateDevAppId,
			CallbackReference:    NilToEmptyUri(curAppContext.CallbackReference),
			AppLocationUpdates:   curAppContext.AppLocationUpdates,
			AppAutoInstantiation: curAppContext.AppAutoInstantiation,
			AppDId:               curAppContext.AppInfo.AppDId,
		}
		err = txn.Insert("AppContextTable", record)
		if err != nil {
			txn.Abort()
			log.Error(err.Error())
			return err
		}

		// Notify listener
		am.notifyListener(curAppContext.ContextId)
	}

	if profiling {
		now := time.Now()
		log.Debug("PutAppContext: ", now.Sub(profilingTimers["PutAppContext"]))
	}

	return nil
}

func (am *DaiMgr) GetAppContextRecord(contextId string) (appContext *AppContext, err error) {
	if profiling {
		profilingTimers["GetAppContextRecord"] = time.Now()
	}

	// Sanity check
	if contextId == "" {
		err = errors.New("Missing ContextId")
		log.Error(err.Error())
		return nil, err
	}

	txn := am.db.Txn(false)
	defer txn.Abort()

	raw, err := txn.First("AppContextTable", "ContextId", contextId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if raw == nil {
		err = errors.New("Wrong ContextId")
		log.Error(err.Error())
		return nil, err
	}
	appContext, err = am.processAppContextRecord(txn, raw.(*AppContextTable))
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	if profiling {
		now := time.Now()
		log.Debug("GetAppContextRecord: ", now.Sub(profilingTimers["GetAppContextRecord"]))
	}

	return appContext, nil
}

func (am *DaiMgr) GetAllAppContextRecord() (appContextList map[string]*AppContext, err error) {
	if profiling {
		profilingTimers["GetAllAppContextRecord"] = time.Now()
	}

	txn := am.db.Txn(false)
	defer txn.Abort()

	it, err := txn.Get("AppContextTable", "ContextId")
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if it != nil {
		// Create AppContext map
		appContextList = make(map[string]*AppContext)

		for obj := it.Next(); obj != nil; obj = it.Next() {
			appContext, err := am.processAppContextRecord(txn, obj.(*AppContextTable))
			if err != nil {
				continue // Skip it
			}
			appContextList[appContext.ContextId] = appContext
		} // End of 'for' statement
	}

	if appContextList == nil {
		err = errors.New("AppInfoList not found")
		return nil, err
	}

	if profiling {
		now := time.Now()
		log.Debug("GetAllAppContextRecord: ", now.Sub(profilingTimers["GetAllAppContextRecord"]))
	}

	return appContextList, nil
}

func (am *DaiMgr) processAppInfoRecord(txn *memdb.Txn, appInfoTable *AppInfoTable) (appInfo *AppInfo, err error) {
	//log.Debug(">>> processAppInfoRecord: ", appDId)

	appInfo = new(AppInfo)
	appInfo.AppDId = appInfoTable.AppDId
	appInfo.AppName = appInfoTable.AppName
	appInfo.AppProvider = appInfoTable.AppProvider
	appInfo.AppSoftVersion = appInfoTable.AppSoftVersion
	appInfo.AppDVersion = appInfoTable.AppDVersion
	appInfo.AppDescription = appInfoTable.AppDescription

	it, err := txn.Get("LocationConstraintsTable", "AppDId", appInfo.AppDId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if it != nil {
		appLocation := make([]LocationConstraintsItem, 0)
		for obj := it.Next(); obj != nil; obj = it.Next() {
			p := obj.(*LocationConstraintsTable)
			var r = LocationConstraintsItem{
				Area:                convertJsonToPolygon(p.Area),
				CivicAddressElement: convertJsonToCivicAddressElement(p.CivicAddressElement),
				CountryCode:         EmptyToNilString(p.CountryCode),
			}
			appLocation = append(appLocation, r)
		} // End of 'for' statement
		appInfo.AppLocation = &appLocation
	}

	raw, err := txn.First("AppCharcsTable", "AppDId", appInfo.AppDId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if raw != nil {
		p := raw.(*AppCharcsTable)
		appInfo.AppCharcs = &AppCharcs{
			Memory:      EmptyToNilUInt32(p.Memory),
			Storage:     EmptyToNilUInt32(p.Storage),
			Latency:     EmptyToNilUInt32(p.Latency),
			Bandwidth:   EmptyToNilUInt32(p.Bandwidth),
			ServiceCont: EmptyToNilUInt32(p.ServiceCont),
		}
	}

	return appInfo, nil
}

func (am *DaiMgr) processAppContextRecord(txn *memdb.Txn, appContextTable *AppContextTable) (appContext *AppContext, err error) {
	//log.Debug(">>> processAppContextRecord: ", appContextTable)

	appContext = new(AppContext)
	appContext.ContextId = appContextTable.ContextId
	appContext.AssociateDevAppId = appContextTable.AssociateDevAppId
	appContext.CallbackReference = new(Uri)
	*appContext.CallbackReference = appContextTable.CallbackReference
	appContext.AppLocationUpdates = appContextTable.AppLocationUpdates
	appContext.AppAutoInstantiation = appContextTable.AppAutoInstantiation
	log.Debug("processAppContextRecord: *appContext.ContextId: ", appContext.ContextId)

	raw, err := txn.First("AppInfoContextTable", "ContextId", appContext.ContextId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if raw != nil {
		p := raw.(*AppInfoContextTable)
		appContext.AppInfo.AppDId = p.AppDId
		appContext.AppInfo.AppName = p.AppName
		appContext.AppInfo.AppProvider = p.AppProvider
		appContext.AppInfo.AppSoftVersion = EmptyToNilString(p.AppSoftVersion)
		appContext.AppInfo.AppDVersion = p.AppDVersion
		appContext.AppInfo.AppDescription = EmptyToNilString(p.AppDescription)
		log.Debug("processAppContextRecord: appContext.AppInfo: ", appContext.AppInfo)
		it, err := txn.Get("UserAppInstanceInfoItemTable", "AppDId", appContext.AppInfo.AppDId)
		if err != nil {
			log.Error(err.Error())
			return nil, err
		}
		if it != nil {
			appContext.AppInfo.UserAppInstanceInfo = make([]UserAppInstanceInfoItem, 0)
			for obj := it.Next(); obj != nil; obj = it.Next() {
				p := obj.(*UserAppInstanceInfoItemTable)
				var r UserAppInstanceInfoItem
				r.AppInstanceId = p.AppInstanceId
				r.ReferenceURI = p.ReferenceURI
				appContext.AppInfo.UserAppInstanceInfo = append(appContext.AppInfo.UserAppInstanceInfo, r)
			} // End of 'for' statement

		}
		log.Debug("processAppContextRecord: appContext.AppInfo.UserAppInstanceInfo: ", appContext.AppInfo.UserAppInstanceInfo)
	}

	return appContext, nil
}

func startProcessCheckTicker() {
	// Make sure ticker is not running
	if processCheckTicker != nil {
		log.Warn("Registration ticker already running")
		return
	}

	// Start registration ticker
	processCheckTicker = time.NewTicker(processCheckExpiry)
	go func() {

		for range processCheckTicker.C {

			if len(appExecEntries) != 0 { // No process running
				for _, appExecEntry := range appExecEntries {

					res, err := pidExists(appExecEntry.cmd.Process.Pid)
					if err != nil {
						log.Error(err.Error())
						continue
					}
					if res == false {
						// Process terminated, delete all entries
						appContextId := strconv.Itoa(appExecEntry.cmd.Process.Pid)
						if appContexts[appContextId].CallbackReference != nil && *appContexts[appContextId].CallbackReference != "" {
							notifyUrl := *appContexts[appContextId].CallbackReference
							daiMgr.DeleteAppContext(appContextId)
							// Notify event
							notifyAppContextDeletion(string(notifyUrl), appContextId)
						}
					}
				} // End of 'for' statement
			}

			continue // Infinite loop till stopProcessCheckTicker is call
		} // End of 'for' statement
	}()
}

func stopProcessCheckTicker() {
	if processCheckTicker != nil {
		log.Info("Stopping App Enablement registration ticker")
		processCheckTicker.Stop()
		processCheckTicker = nil
	}
}

///////////////////////////////////////////////////////////////////////
