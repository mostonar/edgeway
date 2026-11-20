/*
 * Copyright (c) 2024  The AdvantEDGE Authors
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

package vistrafficmgr

import (
	"database/sql"
	"errors"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	_ "github.com/lib/pq"
	"github.com/roymx/viper"
)

// VIS Traffic Manager
type TrafficMgr struct {
	name           string
	namespace      string
	user           string
	pwd            string
	host           string
	port           string
	broker         string
	topic          string
	poa_list       []string
	v2x_notify     func(v2xMessage []byte, v2xType int32, msgProtocolVersion int32, stdOrganization string, longitude *float32, latitude *float32)
	dbName         string
	db             *sql.DB
	connected      bool
	GridFileExists bool
	mutex          sync.Mutex
	poaLoadMap     map[string]*PoaLoads
	message_broker message_broker_interface
	// updateCb  func(string, string)
}

type PoaLoads struct {
	PoaName     string
	Category    string
	Loads       map[string]int32
	AverageLoad int32
}

type CategoryLoads struct {
	Category string
	Loads    map[string]int32
}

type GridMapTable struct {
	area     string
	category string
	grid     string
}

type UuUnicastProvisioningInfoProInfoUuUnicast struct {
	LocationInfo         *LocationInfo
	NeighbourCellInfo    []UuUniNeighbourCellInfo
	V2xApplicationServer *V2xApplicationServer
}
type UuUnicastProvisioningInfoProInfoUuUnicast_list []UuUnicastProvisioningInfoProInfoUuUnicast
type UuMbmsProvisioningInfoProInfoUuMbms struct {
	LocationInfo      *LocationInfo
	NeighbourCellInfo []UuMbmsNeighbourCellInfo
	V2xServerUsd      *V2xServerUsd
}
type UuMbmsProvisioningInfoProInfoUuMbms_list []UuMbmsProvisioningInfoProInfoUuMbms
type Pc5ProvisioningInfoProInfoPc5 struct {
	DstLayer2Id       string
	LocationInfo      *LocationInfo
	NeighbourCellInfo []Pc5NeighbourCellInfo
}
type Pc5ProvisioningInfoProInfoPc5_list []Pc5ProvisioningInfoProInfoPc5
type LocationInfo struct {
	Ecgi    *Ecgi
	GeoArea *LocationInfoGeoArea
}
type UuUniNeighbourCellInfo struct {
	Ecgi    *Ecgi
	FddInfo *FddInfo
	Pci     int32
	Plmn    *Plmn
	TddInfo *TddInfo
}
type UuMbmsNeighbourCellInfo struct {
	Ecgi                    *Ecgi
	FddInfo                 *FddInfo
	MbmsServiceAreaIdentity []string
	Pci                     int32
	Plmn                    *Plmn
	TddInfo                 *TddInfo
}
type Pc5NeighbourCellInfo struct {
	Ecgi        *Ecgi
	Plmn        *Plmn
	SiV2xConfig *SystemInformationBlockType21
}
type V2xApplicationServer struct {
	IpAddress string
	UdpPort   string
}
type V2xServerUsd struct {
	SdpInfo               *V2xServerUsdSdpInfo
	ServiceAreaIdentifier []string
	Tmgi                  *V2xServerUsdTmgi
}
type SystemInformationBlockType21 struct {
}
type Ecgi struct {
	CellId *CellId
	Plmn   *Plmn
}
type CellId struct {
	CellId string
}
type Plmn struct {
	Mcc string
	Mnc string
}
type FddInfo struct {
	DlEarfcn                *int32
	DlTransmissionBandwidth *int32
	UlEarfcn                *int32
	UlTransmissionBandwidth *int32
}
type TddInfo struct {
	Earfcn                *int32
	SubframeAssignment    string
	TransmissionBandwidth *int32
}
type LocationInfoGeoArea struct {
	Latitude  float32
	Longitude float32
}
type V2xServerUsdSdpInfo struct {
	IpMulticastAddress string
	PortNumber         string
}
type V2xServerUsdTmgi struct {
	MbmsServiceId string
	Mcc           string
	Mnc           string
}

var brokerRunning bool = false
var v2xPoaListMap map[string]string = nil
var cellName2CellIdMap map[string]string = nil
var cellId2LocationMap map[string][]float32 = nil
var cellId2CellNameMap map[string]string = nil

// DB Config
const (
	DbHost              = "meep-postgis.default.svc.cluster.local"
	DbPort              = "5432"
	DbUser              = ""
	DbPassword          = ""
	DbDefault           = "postgres"
	DbMaxRetryCount int = 2
)

// Enable profiling
const profiling = false

var profilingTimers map[string]time.Time

const (
	FieldCategory              = "category"
	FieldPoaName               = "poaName"
	FieldZeroToThree           = "0000-0300"
	FieldThreeToSix            = "0300-0600"
	FieldSixToNine             = "0600-0900"
	FieldNineToTwelve          = "0900-1200"
	FieldTwelveToFifteen       = "1200-1500"
	FieldFifteenToEighteen     = "1500-1800"
	FieldEighteenToTwentyOne   = "1800-2100"
	FieldTwentyOneToTwentyFour = "2100-2400"
)

const (
	CategoryCommercial  = "commercial"
	CategoryResidential = "residential"
	CategoryCoastal     = "coastal"
)

// DB Table Names
const (
	GridTable = "grid_map"
)

// Grid Map data
var gridMapData map[string]map[string][]string

// Category-wise Traffic Loads
var categoryLoads = map[string]*CategoryLoads{
	CategoryCommercial: &CategoryLoads{
		Category: CategoryCommercial,
		Loads: map[string]int32{
			FieldZeroToThree:           50,
			FieldThreeToSix:            50,
			FieldSixToNine:             75,
			FieldNineToTwelve:          100,
			FieldTwelveToFifteen:       125,
			FieldFifteenToEighteen:     100,
			FieldEighteenToTwentyOne:   75,
			FieldTwentyOneToTwentyFour: 50,
		},
	},
	CategoryResidential: &CategoryLoads{
		Category: CategoryResidential,
		Loads: map[string]int32{
			FieldZeroToThree:           125,
			FieldThreeToSix:            125,
			FieldSixToNine:             100,
			FieldNineToTwelve:          75,
			FieldTwelveToFifteen:       50,
			FieldFifteenToEighteen:     50,
			FieldEighteenToTwentyOne:   125,
			FieldTwentyOneToTwentyFour: 125,
		},
	},
	CategoryCoastal: &CategoryLoads{
		Category: CategoryCoastal,
		Loads: map[string]int32{
			FieldZeroToThree:           25,
			FieldThreeToSix:            25,
			FieldSixToNine:             50,
			FieldNineToTwelve:          25,
			FieldTwelveToFifteen:       50,
			FieldFifteenToEighteen:     75,
			FieldEighteenToTwentyOne:   50,
			FieldTwentyOneToTwentyFour: 25,
		},
	},
}

var timeWindows = map[int32]string{
	0:  FieldZeroToThree,
	1:  FieldZeroToThree,
	2:  FieldZeroToThree,
	3:  FieldThreeToSix,
	4:  FieldThreeToSix,
	5:  FieldThreeToSix,
	6:  FieldSixToNine,
	7:  FieldSixToNine,
	8:  FieldSixToNine,
	9:  FieldNineToTwelve,
	10: FieldNineToTwelve,
	11: FieldNineToTwelve,
	12: FieldTwelveToFifteen,
	13: FieldTwelveToFifteen,
	14: FieldTwelveToFifteen,
	15: FieldFifteenToEighteen,
	16: FieldFifteenToEighteen,
	17: FieldFifteenToEighteen,
	18: FieldEighteenToTwentyOne,
	19: FieldEighteenToTwentyOne,
	20: FieldEighteenToTwentyOne,
	21: FieldTwentyOneToTwentyFour,
	22: FieldTwentyOneToTwentyFour,
	23: FieldTwentyOneToTwentyFour,
}

// Profiling init
func init() {
	if profiling {
		profilingTimers = make(map[string]time.Time)
	}
}

// NewTrafficMgr - Creates and initializes a new VIS Traffic Manager
func NewTrafficMgr(name string, namespace string, user string, pwd string, host string, port string, broker string, topic string, poa_list []string, v2x_notify func(v2xMessage []byte, v2xType int32, msgProtocolVersion int32, stdOrganization string, longitude *float32, latitude *float32)) (tm *TrafficMgr, err error) {
	if name == "" {
		err = errors.New("Missing connector name")
		return nil, err
	}

	// Create new Traffic Manager
	tm = new(TrafficMgr)
	tm.name = name
	if namespace != "" {
		tm.namespace = namespace
	} else {
		tm.namespace = "default"
	}
	tm.user = user
	tm.pwd = pwd
	tm.host = host
	tm.port = port
	tm.broker = broker
	tm.topic = topic
	tm.poa_list = poa_list
	tm.v2x_notify = v2x_notify
	tm.poaLoadMap = map[string]*PoaLoads{}

	// Connect to Postgis DB
	for retry := 0; retry <= DbMaxRetryCount; retry++ {
		tm.db, err = tm.connectDB("", tm.user, tm.pwd, tm.host, tm.port)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Error("Failed to connect to postgis DB with err: ", err.Error())
		return nil, err
	}
	defer tm.db.Close()

	// Create sandbox DB if it does not exist
	// Use format: '<namespace>_<name>' & replace dashes with underscores
	tm.dbName = strings.ToLower(strings.Replace(namespace+"_"+name, "-", "_", -1))

	// Ignore DB creation error in case it already exists.
	// Failure will occur at DB connection if DB was not successfully created.
	_ = tm.CreateDb(tm.dbName)

	// Close connection to postgis DB
	_ = tm.db.Close()

	// Connect with sandbox-specific DB
	tm.db, err = tm.connectDB(tm.dbName, user, pwd, host, port)
	if err != nil {
		log.Error("Failed to connect to sandbox DB with err: ", err.Error())
		return nil, err
	}

	// Open grid map file
	gridMapData, tm.GridFileExists, err = getGridMapConfig()
	if err != nil {
		log.Error("Failed to open grid map file with err: ", err.Error())
		return tm, err
	}

	log.Info("Postgis Connector successfully created")
	tm.connected = true
	return tm, nil
}

func (tm *TrafficMgr) connectDB(dbName, user, pwd, host, port string) (db *sql.DB, err error) {
	// Set default values if none provided
	if dbName == "" {
		dbName = DbDefault
	}
	if host == "" {
		host = DbHost
	}
	if port == "" {
		port = DbPort
	}
	log.Debug("Connecting to Postgis DB [", dbName, "] at addr [", host, ":", port, "]")

	// Open postgis DB
	connStr := "user=" + user + " password=" + pwd + " dbname=" + dbName + " host=" + host + " port=" + port + " sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Warn("Failed to connect to Postgis DB with error: ", err.Error())
		return nil, err
	}

	// Make sure connection is up
	err = db.Ping()
	if err != nil {
		log.Warn("Failed to ping Postgis DB with error: ", err.Error())
		db.Close()
		return nil, err
	}

	log.Info("Connected to Postgis DB [", dbName, "]")
	return db, nil
}

// func (tm *TrafficMgr) SetListener(listener func(string, string)) error {
// 	tm.updateCb = listener
// 	return nil
// }

// func (tm *TrafficMgr) notifyListener(cbType string, assetName string) {
// 	if tm.updateCb != nil {
// 		go tm.updateCb(cbType, assetName)
// 	}
// }

// DeleteTrafficMgr -
func (tm *TrafficMgr) DeleteTrafficMgr() (err error) {

	if tm.db == nil {
		err = errors.New("Traffic Manager database not initialized")
		log.Error(err.Error())
		return err
	}

	// Close connection to sandbox-specific DB
	_ = tm.db.Close()

	// Connect to Postgis DB
	tm.db, err = tm.connectDB("", tm.user, tm.pwd, tm.host, tm.port)
	if err != nil {
		log.Error("Failed to connect to postgis DB with err: ", err.Error())
		return err
	}
	defer tm.db.Close()

	// Destroy sandbox database
	_ = tm.DestroyDb(tm.dbName)

	tm.StopV2xMessageBrokerServer()

	return nil
}

// CreateDb -- Create new DB with provided name
func (tm *TrafficMgr) CreateDb(name string) (err error) {
	_, err = tm.db.Exec("CREATE DATABASE " + name)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	log.Info("Created database: " + name)
	return nil
}

// DestroyDb -- Destroy DB with provided name
func (tm *TrafficMgr) DestroyDb(name string) (err error) {
	_, err = tm.db.Exec("DROP DATABASE " + name)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	log.Info("Destroyed database: " + name)
	return nil
}

func getGridMapConfig() (gridData map[string]map[string][]string, gridFile bool, err error) {
	// Read grid map from grid map file
	gridMapFile := "/grid_map.yaml"
	gridMap := viper.New()
	gridMap.SetConfigFile(gridMapFile)
	err = gridMap.ReadInConfig()
	if err != nil {
		return nil, false, err
	}

	var config map[string]map[string][]string
	err = gridMap.Unmarshal(&config)
	if err != nil {
		return nil, false, err
	}
	return config, true, nil
}

func (tm *TrafficMgr) CreateTables() (err error) {
	_, err = tm.db.Exec("CREATE EXTENSION IF NOT EXISTS postgis")
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Grid Table
	_, err = tm.db.Exec(`CREATE TABLE IF NOT EXISTS ` + GridTable + ` (
		area            varchar(100)            NOT NULL,
		category				varchar(100)						NOT NULL,
		grid						geometry(POLYGON,4326),
		PRIMARY KEY (area)
	)`)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	log.Info("Created Grids table: ", GridTable)

	return nil
}

// DeleteTables - Delete all postgis traffic tables
func (tm *TrafficMgr) DeleteTables() (err error) {
	_ = tm.DeleteTable(GridTable)
	return nil
}

// DeleteTable - Delete postgis table with provided name
func (tm *TrafficMgr) DeleteTable(tableName string) (err error) {
	_, err = tm.db.Exec("DROP TABLE IF EXISTS " + tableName)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	log.Info("Deleted table: " + tableName)
	return nil
}

// CreateGridMap - Create new Grid Map
func (tm *TrafficMgr) CreateGridMap(area string, category string, grid string) (err error) {
	if profiling {
		profilingTimers["CreateGridMap"] = time.Now()
	}

	// Validate input
	if area == "" {
		return errors.New("Missing area name")
	}
	if category == "" {
		return errors.New("Missing category name")
	}
	if grid == "" {
		return errors.New("Missing grid polygon data")
	}

	// Create Grid Map entry
	query := `INSERT INTO ` + GridTable +
		` (area, category, grid)
			VALUES ($1, $2, ST_GeomFromEWKT('SRID=4326;POLYGON(` + grid + `)'))`
	_, err = tm.db.Exec(query, area, category)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Notify listener
	// tm.notifyListener(TypePoa, name)

	if profiling {
		now := time.Now()
		log.Debug("CreateGridMap: ", now.Sub(profilingTimers["CreateGridMap"]))
	}
	return nil
}

// CreatePoaLoad - Create new POA Load
func (tm *TrafficMgr) CreatePoaLoad(poaName string, category string) (err error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Validate input
	if poaName == "" {
		return errors.New("Missing POA Name")
	}
	if category == "" {
		return errors.New("Missing category name")
	}

	// Get Load entry from Categories Table
	categoryLoads, found := categoryLoads[category]
	if !found {
		return errors.New("Invalid category name: " + category)
	}

	// Create POA loads entry
	poaLoads := &PoaLoads{
		PoaName:  poaName,
		Category: category,
		Loads:    map[string]int32{},
	}
	// Copy category loads & calculate average load
	if len(categoryLoads.Loads) > 0 {
		var loadSum int32 = 0
		for k, v := range categoryLoads.Loads {
			poaLoads.Loads[k] = v
			loadSum += v
		}
		poaLoads.AverageLoad = loadSum / int32(len(poaLoads.Loads))
	}
	log.Info("Created loads table for ", poaName, " (", category, "): ", poaLoads.Loads, ", Average: ", poaLoads.AverageLoad)

	// Add POA loads to map
	tm.poaLoadMap[poaName] = poaLoads
	return nil
}

// GetPoaLoad - Get POA Load information
func (tm *TrafficMgr) GetPoaLoad(poaName string) (poaLoads *PoaLoads, err error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Validate input
	if poaName == "" {
		err = errors.New("Missing POA Name")
		return nil, err
	}

	// Get POA loads
	poaLoads = tm.poaLoadMap[poaName]
	if poaLoads == nil {
		err = errors.New("POA loads not found: " + poaName)
		return nil, err
	}
	return poaLoads, nil
}

// GetAllPoaLoad - Get all POA information
func (tm *TrafficMgr) GetAllPoaLoad() (poaLoadMap map[string]*PoaLoads, err error) {
	return tm.poaLoadMap, nil
}

// DeleteAllPoaLoads - Delete all POA entries
func (tm *TrafficMgr) DeleteAllPoaLoad() (err error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Reset poa loads
	tm.poaLoadMap = map[string]*PoaLoads{}
	return nil
}

// GetGridMap - Get GridMap information
func (tm *TrafficMgr) GetGridMap(area string) (gridMaps *GridMapTable, err error) {
	if profiling {
		profilingTimers["GetGridMap"] = time.Now()
	}

	// Validate input
	if area == "" {
		err = errors.New("Missing area Name")
		return nil, err
	}

	// Get GridMap entry
	var rows *sql.Rows
	rows, err = tm.db.Query(`
		SELECT area, category, grid
		FROM `+GridTable+`
		WHERE area = ($1)`, area)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	defer rows.Close()

	// Scan result
	for rows.Next() {
		gridMaps = new(GridMapTable)
		err = rows.Scan(
			&gridMaps.area,
			&gridMaps.category,
			&gridMaps.grid,
		)
		if err != nil {
			log.Error(err.Error())
			return nil, err
		}
	}
	err = rows.Err()
	if err != nil {
		log.Error(err)
	}

	// Return error if not found
	if gridMaps == nil {
		err = errors.New("GridMap Load not found: " + area)
		return nil, err
	}

	if profiling {
		now := time.Now()
		log.Debug("GetGridMap: ", now.Sub(profilingTimers["GetGridMap"]))
	}
	return gridMaps, nil
}

// GetAllGridMap - Get GridMap information
func (tm *TrafficMgr) GetAllGridMap() (gridMaps map[string]*GridMapTable, err error) {
	if profiling {
		profilingTimers["GetAllGridMap"] = time.Now()
	}

	// Create Category map
	gridMaps = make(map[string]*GridMapTable)

	// Get Category Load entry
	var rows *sql.Rows
	rows, err = tm.db.Query(`SELECT area, category, grid FROM ` + GridTable)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	defer rows.Close()

	// Scan results
	for rows.Next() {

		gridMapItem := new(GridMapTable)
		err = rows.Scan(
			&gridMapItem.area,
			&gridMapItem.category,
			&gridMapItem.grid,
		)

		// Add Grid item to map
		gridMaps[gridMapItem.area] = gridMapItem
	}
	err = rows.Err()
	if err != nil {
		log.Error(err)
	}

	if profiling {
		now := time.Now()
		log.Debug("GetAllGridMap: ", now.Sub(profilingTimers["GetAllGridMap"]))
	}

	return gridMaps, nil
}

// DeleteAllGridMap - Delete all GridMap entries
func (tm *TrafficMgr) DeleteAllGridMap() (err error) {
	if profiling {
		profilingTimers["DeleteAllGridMap"] = time.Now()
	}

	_, err = tm.db.Exec(`DELETE FROM ` + GridTable)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	if profiling {
		now := time.Now()
		log.Debug("DeleteAllGridMap: ", now.Sub(profilingTimers["DeleteAllGridMap"]))
	}
	return nil
}

// PopulateGridMapTable - Populate the grid_map table
func (tm *TrafficMgr) PopulateGridMapTable() (err error) {
	if profiling {
		profilingTimers["PopulateGridMapTable"] = time.Now()
	}

	// Get grid map from YAML file
	for category, areas := range gridMapData {
		for area, points := range areas {
			polygonStr := "("
			for i, pointStr := range points {
				if i != len(points)-1 {
					polygonStr = polygonStr + pointStr + ", "
				} else {
					polygonStr = polygonStr + pointStr + ")"
				}
			}
			err = tm.CreateGridMap(area, category, polygonStr)
			if err != nil {
				log.Error(err.Error())
				return err
			}
		}
	}

	if profiling {
		now := time.Now()
		log.Debug("PopulateGridMapTable: ", now.Sub(profilingTimers["PopulateGridMapTable"]))
	}
	return nil
}

// GetPoaCategory - Get the category for a PoA
func (tm *TrafficMgr) GetPoaCategory(longitude float32, latitude float32) (category string, err error) {
	if profiling {
		profilingTimers["GetPoaCategory"] = time.Now()
	}

	coordinates := "(" + strconv.FormatFloat(float64(longitude), 'E', -1, 32) + " " + strconv.FormatFloat(float64(latitude), 'E', -1, 32) + ")"

	dbQuery := "SELECT category FROM " + GridTable + " WHERE ST_Contains(" + GridTable + ".grid, 'SRID=4326;POINT" + coordinates + "');"

	var rows *sql.Rows
	rows, err = tm.db.Query(dbQuery)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	defer rows.Close()

	category = ""

	if rows.Next() {
		err = rows.Scan(&category)
		if err != nil {
			log.Error(err.Error())
			return category, err
		}
		return category, nil
	}
	err = rows.Err()
	if err != nil {
		log.Error(err)
	}
	return category, err
}

// PopulatePoaLoad - Populate the Traffic Load table
func (tm *TrafficMgr) PopulatePoaLoad(poaNameList []string, gpsCoordinates [][]float32) (err error) {
	log.Debug(">>> PopulatePoaLoad: poaNameList: ", poaNameList)
	log.Debug(">>> PopulatePoaLoad: gpsCoordinates: ", gpsCoordinates)

	// Validate input
	if poaNameList == nil {
		err = errors.New("Missing POA Name List")
		return err
	}
	if gpsCoordinates == nil {
		err = errors.New("Missing GPS coordinates")
		return err
	}

	// Get POA loads for each POA
	for i, poaName := range poaNameList {
		// Get POA category from locaion & grid map
		poaLongitude := gpsCoordinates[i][0]
		poaLatitude := gpsCoordinates[i][1]
		category, err := tm.GetPoaCategory(poaLongitude, poaLatitude)
		if err != nil {
			log.Error(err.Error())
			return err
		}

		// Set POA load
		err = tm.CreatePoaLoad(poaName, category)
		if err != nil {
			log.Error(err.Error())
			return err
		}
	}

	return nil
}

// Returns Predicted QoS in terms of RSRQ and RSRP values based on Traffic Load patterns
func (tm *TrafficMgr) PredictQosPerTrafficLoad(hour int32, inRsrp int32, inRsrq int32, poaName string) (outRsrp int32, outRsrq int32, err error) {
	// Validate input
	if hour > 24 {
		err = errors.New("Invalid hour value")
		return 0, 0, err
	}
	if poaName == "" {
		err = errors.New("Missing POA Name")
		return 0, 0, err
	}

	// Get time range for DB query
	timeRange, found := timeWindows[hour]
	if !found {
		err = errors.New("Invalid hour value")
		return 0, 0, err
	}

	// Get predicted load for a given PoA in a desired time slot from the traffic patterns table
	log.Debug("Obtaining traffic load pattern of POA " + poaName + " for the time range: " + timeRange)
	poaLoads, err := tm.GetPoaLoad(poaName)
	if err != nil {
		return 0, 0, err
	}
	var predictedUserTraffic int32
	predictedUserTraffic, found = poaLoads.Loads[timeRange]
	if !found {
		err = errors.New("Could not find estimated user load")
		return 0, 0, err
	}

	// Find reduced signal strength as a function of number of users in the area
	outRsrp, outRsrq, err = findReducedSignalStrength(inRsrp, inRsrq, predictedUserTraffic, poaLoads.AverageLoad)
	return outRsrp, outRsrq, err
}

// Returns reduced signal strength based on the deviation in predicted user traffic from average user load in that area
// The RSRP/RSRP values are reduced proportional to the difference between estimated users and average user traffic in a given POA
// Assumption: the RSRP/RSRP values remain unchanged for average POA traffic
func findReducedSignalStrength(inRsrp int32, inRsrq int32, users int32, averageLoad int32) (redRsrp int32, redRsrq int32, err error) {

	// Case: crowded area
	if users > averageLoad {
		redRsrp = int32(math.Max(float64(float32(inRsrp)*(float32(averageLoad)/float32(users))), float64(40)))
		redRsrq = int32(math.Max(float64(float32(inRsrq)*(float32(averageLoad)/float32(users))), float64(0)))

		return redRsrp, redRsrq, nil

	} else {
		// no change in RSRP and RSRQ values
		return inRsrp, inRsrq, nil
	}
}

/*
 * GetInfoUuUnicast process the uu_unicast_provisioning_info GETT request and sends the response
 * @param {struct} params HTTP request parameters
 * @param {struct} num_item contains the number of parameters
 * @return {struct} an initialized UuUnicastProvisioningInfoProInfoUuUnicast_list data structure
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.3.3.1 GET
 */
func (tm *TrafficMgr) GetInfoUuUnicast(params []string, num_item int) (proInfoUuUnicast UuUnicastProvisioningInfoProInfoUuUnicast_list, err error) {
	log.Debug(">>> GetInfoUuUnicast: params: ", params)
	log.Debug(">>> GetInfoUuUnicast: num_item: ", num_item)

	proInfoUuUnicast = make([]UuUnicastProvisioningInfoProInfoUuUnicast, num_item)
	if params[0] == "ecgi" {
		for i := 1; i <= num_item; i++ { // Same brocker for all Zone.
			// TODO To be enhance to have one broker per zone confgured in Zone node
			log.Info("GetInfoUuUnicast: Processing index #", i)

			// Find the ecgi in table
			var location_map []float32
			if val, ok := cellId2LocationMap[params[i]]; ok {
				location_map = val
				log.Info("GetInfoUuUnicast: location= ", location_map)
			} else {
				err = errors.New("Cannot find cell " + params[i])
				log.Error(err.Error())
				continue //return nil, err
			}
			log.Info("GetInfoUuUnicast: location= ", location_map)

			ecgi, err := build_ecgi(params[i])
			if err != nil {
				log.Error(err.Error())
				continue //return nil, err
			}

			// FIXME FSCOM How to manage neighbour cellIs
			uuUniNeighbourCellInfo := make([]UuUniNeighbourCellInfo, 0)
			//uuUniNeighbourCellInfo := make([]UuUniNeighbourCellInfo, 1)
			//uuUniNeighbourCellInfo[0] = UuUniNeighbourCellInfo{&ecgi, nil, 0, &plmn, nil}

			var v2xApplicationServer *V2xApplicationServer = nil
			if _, found := cellId2CellNameMap[params[i]]; found {
				u, err := url.ParseRequestURI(tm.broker)
				if err != nil {
					log.Error(err.Error())
					return nil, err
				}
				//log.Info("url:%v - scheme:%v - host:%v - Path:%v - Port:%s", u, u.Scheme, u.Hostname(), u.Path, u.Port())
				v2xApplicationServer = &V2xApplicationServer{
					IpAddress: u.Hostname(),
					UdpPort:   u.Port(),
				}
			}

			locationInfoGeoArea := &LocationInfoGeoArea{location_map[1], location_map[0]}
			locationInfo := &LocationInfo{&ecgi, locationInfoGeoArea}

			proInfoUuUnicast[i-1] = UuUnicastProvisioningInfoProInfoUuUnicast{locationInfo, uuUniNeighbourCellInfo, v2xApplicationServer}
		} // End of 'for' statement
	} else if params[0] == "latitude" {
		log.Info("GetInfoUuUnicast: Params is latitude")

		// FIXME FSCOM Add logic based on position:
		//	1) Find the POA closest to the position
		//  2) Verify if it provides V2X services
		//  3) Uodate the data structures accordingly
		for i := 1; i <= num_item; i++ {
			log.Info("GetInfoUuUnicast: Processing index #", i)
			log.Info("GetInfoUuUnicast: params[i]: ", params[i])
			log.Info("GetInfoUuUnicast: params[i + num_item + 1]: ", params[i+num_item+1])

			lat, err := strconv.ParseFloat(params[i], 32)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			long, err := strconv.ParseFloat(params[i+num_item+1], 32)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			var min_distance float64 = math.MaxFloat64
			var selected_ecgi string = ""
			for idx, coord := range cellId2LocationMap {
				dist := distance_gps_distance_in_meters(float64(lat), float64(long), float64(coord[1]), float64(coord[0]))
				if dist < min_distance {
					min_distance = dist
					selected_ecgi = idx
					log.Info("GetInfoUuUnicast: min_distance: ", min_distance)
					log.Info("GetInfoUuUnicast: selected_ecgi: ", selected_ecgi)
				}
			} // End of 'for'statement
			if selected_ecgi == "" {
				err = errors.New("Failed to get the closest cell")
				log.Error(err.Error())
				return nil, err
			}

			ecgi, err := build_ecgi(selected_ecgi)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}

			locationInfoGeoArea := &LocationInfoGeoArea{cellId2LocationMap[selected_ecgi][1], cellId2LocationMap[selected_ecgi][0]}
			locationInfo := &LocationInfo{&ecgi, locationInfoGeoArea}
			var v2xApplicationServer *V2xApplicationServer = nil
			u, err := url.ParseRequestURI(tm.broker)
			log.Info("GetInfoUuUnicast: u: ", u)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			log.Info("GetInfoUuUnicast: url:%v\nscheme:%v host:%v Path:%v Port:%s", u, u.Scheme, u.Hostname(), u.Path, u.Port())
			v2xApplicationServer = &V2xApplicationServer{
				IpAddress: u.Hostname(),
				UdpPort:   u.Port(),
			}
			log.Info("GetInfoUuUnicast: v2xApplicationServer: ", *v2xApplicationServer)
			proInfoUuUnicast[i-1] = UuUnicastProvisioningInfoProInfoUuUnicast{locationInfo, make([]UuUniNeighbourCellInfo, 0), v2xApplicationServer}
		} // End of 'for' statement
	} else {
		err = errors.New("GetInfoUuUnicast: Invalid parameter: " + params[0])
		return nil, err
	}

	log.Debug("<<< GetInfoUuUnicast: proInfoUuUnicast= ", proInfoUuUnicast)
	return proInfoUuUnicast, nil
}

/*
 * build_ecgi build an Ecgi data structure based on the ECGI name
 * @param {string} ecgi_str the ECGI name
 * @return {struct} an initialized Ecgi data structure
 */
func build_ecgi(ecgi_str string) (ecgi Ecgi, err error) {
	log.Debug(">>> build_ecgi: ecgi_str= ", ecgi_str)

	ecgi_num, err := strconv.Atoi(ecgi_str)
	if err != nil {
		log.Error(err.Error())
		return ecgi, err
	}
	log.Info("build_ecgi: ecgi_num= ", ecgi_num)

	// Extract Poa CellId according to v2x_msg GS MEC 030 Clause 6.5.5 Type: Ecgi
	TwentyEigthBits := 0xFFFFFFF //  TS 36.413: E-UTRAN Cell Identity (ECI) and E-UTRAN Cell Global Identification (ECGI)
	eci := ecgi_num & TwentyEigthBits
	log.Info("build_ecgi: eci= ", int(eci))
	// Extract Poa Plmn according to v2x_msg GS MEC 030 Clause 6.5.4 Type: Plmn
	plmn_num := int(ecgi_num >> 28)
	//log.Info("build_ecgi: plmn= ", plmn_num)
	//mcc_num := int((plmn_num / 1000) & 0xFFFFFF)
	//mnc_num := int((plmn_num - mcc_num * 1000) & 0xFFFFFF)
	mcc_num := int(plmn_num / 1000)
	mnc_num := int(plmn_num - mcc_num*1000)
	//log.Info("build_ecgi: mcc_num= ", mcc_num)
	//log.Info("build_ecgi: mnc_num= ", mnc_num)

	plmn := Plmn{Mcc: strconv.Itoa(int(mcc_num)), Mnc: strconv.Itoa(int(mnc_num))}
	ecgi = Ecgi{
		CellId: &CellId{CellId: strconv.Itoa(int(eci))},
		Plmn:   &plmn,
	}

	log.Debug("<<< build_ecgi: ecgi= ", ecgi)
	return ecgi, nil
}

const EarthRadius float64 = 6378.1370
const Degrees2Rads float64 = math.Pi / float64(180.0)

/*
 * distance_gps_distance_in_km compute the heavyside distance in km between two GPS points (faster than addressing meep-gis-asset-mgr)
 * @param {float64} lat1 the latitude of point #1
 * @param {float64} long1 the longitude of point #1
 * @param {float64} lat2 the latitude of point #2
 * @param {float64} long2 the longitude of point #2
 * @return {float64} The distance in meter
 */
func distance_gps_distance_in_meters(lat1 float64, long1 float64, lat2 float64, long2 float64) float64 {
	return float64(1000.0) * distance_gps_distance_in_km(lat1, long1, lat2, long2)
}

/*
 * distance_gps_distance_in_km compute the heavyside distance in km between two GPS points (faster than addressing meep-gis-asset-mgr)
 * @param {float64} lat1 the latitude of point #1
 * @param {float64} long1 the longitude of point #1
 * @param {float64} lat2 the latitude of point #2
 * @param {float64} long2 the longitude of point #2
 * @return {float64} The distance in kmeter
 */
func distance_gps_distance_in_km(lat1 float64, long1 float64, lat2 float64, long2 float64) float64 {
	log.Debug(">>> distance_gps_distance_in_km")
	dlong := float64((long2 - long1) * Degrees2Rads)
	dlat := float64((lat2 - lat1) * Degrees2Rads)
	a := math.Pow(math.Sin(dlat/2.0), 2.0) + math.Cos(lat1*Degrees2Rads)*math.Cos(lat2*Degrees2Rads)*math.Pow(math.Sin(dlong/2.0), 2.0)
	c := 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))
	d := EarthRadius * c
	log.Debug("<<< distance_gps_distance_in_km: ", d)
	return d
}

/*
 * GetInfoUuMbmscast process the uu_mbms_provisioning_info GETT request and sends the response
 * @param {struct} params HTTP request parameters
 * @param {struct} num_item contains the number of parameters
 * @return {struct} an initialized UuMbmsProvisioningInfoProInfoUuMbms_list data structure
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.4.3.1 GET
 */
func (tm *TrafficMgr) GetInfoUuMbmscast(params []string, num_item int) (proInfoUuMbmscast UuMbmsProvisioningInfoProInfoUuMbms_list, err error) {
	log.Debug(">>> GetInfoUuMbmscast: params: ", params)
	log.Debug(">>> GetInfoUuMbmscast: num_item: ", num_item)

	proInfoUuMbmscast = make([]UuMbmsProvisioningInfoProInfoUuMbms, num_item)
	if params[0] == "ecgi" {
		for i := 1; i <= num_item; i++ {
			log.Info("GetInfoUuMbmscast: Processing index #", i)

			// Find the ecgi in table
			var location_map []float32
			if val, ok := cellId2LocationMap[params[i]]; ok {
				location_map = val
				log.Info("GetInfoUuUnicast: location= ", location_map)
			} else {
				err = errors.New("Cannot find cell " + params[i])
				log.Error(err.Error())
				return nil, err
			}
			log.Info("GetInfoUuUnicast: location= ", location_map)

			ecgi, err := build_ecgi(params[i])
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}

			// FIXME FSCOM How to manage neighbour cellIs
			uuMbmsNeighbourCellInfo := make([]UuMbmsNeighbourCellInfo, 0)
			// uuMbmsNeighbourCellInfo := make([]UuMbmsNeighbourCellInfo, 1)
			// uuMbmsNeighbourCellInfo[0] = UuMbmsNeighbourCellInfo{&ecgi, nil, make([]string, 0), 0, &plmn, nil}

			// FIXME FSCOM How to manage V2xServerUsd
			var v2xServerUsd *V2xServerUsd = nil

			locationInfoGeoArea := &LocationInfoGeoArea{location_map[1], location_map[0]}
			locationInfo := &LocationInfo{&ecgi, locationInfoGeoArea}

			proInfoUuMbmscast[i-1] = UuMbmsProvisioningInfoProInfoUuMbms{locationInfo, uuMbmsNeighbourCellInfo, v2xServerUsd}
		} // End of 'for' statement
	} else if params[0] == "latitude" {
		log.Info("GetInfoUuMbmscast: Params is latitude")

		// FIXME Add logic based on position:
		//	1) Find the POA closest to the position
		//  2) Verify if it provides V2X services
		//  3) Uodate the data structures accordingly
		for i := 1; i <= num_item; i++ {
			log.Info("GetInfoUuMbmscast: Processing index #", i)
			log.Info("GetInfoUuMbmscast: params[i]: ", params[i])
			log.Info("GetInfoUuMbmscast: params[i + num_item + 1]: ", params[i+num_item+1])

			lat, err := strconv.ParseFloat(params[i], 32)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			long, err := strconv.ParseFloat(params[i+num_item+1], 32)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			var min_distance float64 = math.MaxFloat64
			var selected_ecgi string = ""
			for idx, coord := range cellId2LocationMap {
				dist := distance_gps_distance_in_meters(float64(lat), float64(long), float64(coord[1]), float64(coord[0]))
				if dist < min_distance {
					min_distance = dist
					selected_ecgi = idx
					log.Info("GetInfoUuMbmscast: min_distance: ", min_distance)
					log.Info("GetInfoUuMbmscast: selected_ecgi: ", selected_ecgi)
				}
			} // End of 'for'statement
			if selected_ecgi == "" {
				err = errors.New("Failed to get the closest cell")
				log.Error(err.Error())
				return nil, err
			}

			ecgi, err := build_ecgi(selected_ecgi)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}

			var v2xServerUsd *V2xServerUsd = nil
			// FIXME FSCOM
			// if _, found := cellId2CellNameMap[params[i]]; found {
			// 	u, err := url.ParseRequestURI(tm.broker)
			// 	if err != nil {
			// 		log.Error(err.Error())
			// 		return nil, err
			// 	}
			// 	log.Info("url:%v - scheme:%v - host:%v - Path:%v - Port:%s", u, u.Scheme, u.Hostname(), u.Path, u.Port())
			// 	V2xServerUsd = &V2xServerUsd{
			// 		IpAddress: u.Hostname(),
			// 		UdpPort:   u.Port(),
			// 	}
			// }

			locationInfoGeoArea := &LocationInfoGeoArea{cellId2LocationMap[selected_ecgi][1], cellId2LocationMap[selected_ecgi][0]}
			locationInfo := &LocationInfo{&ecgi, locationInfoGeoArea}

			proInfoUuMbmscast[i-1] = UuMbmsProvisioningInfoProInfoUuMbms{locationInfo, make([]UuMbmsNeighbourCellInfo, 0), v2xServerUsd}
		} // End of 'for' statement
	} else {
		err = errors.New("GetInfoUuMbmscast: Invalid parameter: " + params[0])
		proInfoUuMbmscast = nil
	}

	log.Info("GetInfoUuMbmscast: proInfoUuMbmscast= ", proInfoUuMbmscast)
	return proInfoUuMbmscast, nil
}

/*
 * GetInfoPc5 process the uu_mbms_provisioning_info GETT request and sends the response
 * @param {struct} params HTTP request parameters
 * @param {struct} num_item contains the number of parameters
 * @return {struct} an initialized Pc5ProvisioningInfoProInfoPc5_list data structure
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.4.3.1 GET
 */
func (tm *TrafficMgr) GetInfoPc5(params []string, num_item int) (proInfoPc5 Pc5ProvisioningInfoProInfoPc5_list, err error) {
	log.Debug(">>> GetInfoPc5: params: ", params)
	log.Debug(">>> GetInfoPc5: num_item: ", num_item)

	proInfoPc5 = make([]Pc5ProvisioningInfoProInfoPc5, num_item)
	if params[0] == "ecgi" {
		for i := 1; i <= num_item; i++ {
			// TODO To be enhance to have one broker per zone confgured in Zone node
			log.Info("GetInfoPc5: Processing index #", i)

			// Find the ecgi in table
			var location_map []float32
			if val, ok := cellId2LocationMap[params[i]]; ok {
				location_map = val
				log.Info("GetInfoPc5: location= ", location_map)
			} else {
				err = errors.New("Cannot find cell " + params[i])
				log.Error(err.Error())
				return nil, err
			}
			log.Info("GetInfoPc5: location= ", location_map)

			ecgi, err := build_ecgi(params[i])
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}

			// FIXME FSCOM How to manage neighbour cellIs
			pc5NeighbourCellInfo := make([]Pc5NeighbourCellInfo, 0)
			//pc5NeighbourCellInfo := make([]Pc5NeighbourCellInfo, 1)
			//pc5NeighbourCellInfo[0] = Pc5NeighbourCellInfo{&ecgi, &plmn, nil}

			//var siV2xConfig *SystemInformationBlockType21 = new(SystemInformationBlockType21)

			locationInfoGeoArea := &LocationInfoGeoArea{location_map[1], location_map[0]}
			locationInfo := &LocationInfo{&ecgi, locationInfoGeoArea}

			proInfoPc5[i-1] = Pc5ProvisioningInfoProInfoPc5{"dstLayer2Id", locationInfo, pc5NeighbourCellInfo}
		} // End of 'for' statement
	} else if params[0] == "latitude" {
		log.Info("GetInfoPc5: Params is latitude")
		// FIXME Add logic based on position:
		//	1) Find the POA closest to the position
		//  2) Verify if it provides V2X services
		//  3) Uodate the data structures accordingly
		for i := 1; i <= num_item; i++ {
			log.Info("GetInfoPc5: Processing index #", i)
			log.Info("GetInfoPc5: params[i]: ", params[i])
			log.Info("GetInfoPc5: params[i + num_item + 1]: ", params[i+num_item+1])

			lat, err := strconv.ParseFloat(params[i], 32)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			long, err := strconv.ParseFloat(params[i+num_item+1], 32)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}
			var min_distance float64 = math.MaxFloat64
			var selected_ecgi string = ""
			for idx, coord := range cellId2LocationMap {
				dist := distance_gps_distance_in_meters(float64(lat), float64(long), float64(coord[1]), float64(coord[0]))
				if dist < min_distance {
					min_distance = dist
					selected_ecgi = idx
					log.Info("GetInfoPc5: min_distance: ", min_distance)
					log.Info("GetInfoPc5: selected_ecgi: ", selected_ecgi)
				}
			} // End of 'for'statement
			if selected_ecgi == "" {
				err = errors.New("Failed to get the closest cell")
				log.Error(err.Error())
				return nil, err
			}

			ecgi, err := build_ecgi(selected_ecgi)
			if err != nil {
				log.Error(err.Error())
				return nil, err
			}

			locationInfoGeoArea := &LocationInfoGeoArea{cellId2LocationMap[selected_ecgi][1], cellId2LocationMap[selected_ecgi][0]}
			locationInfo := &LocationInfo{&ecgi, locationInfoGeoArea}

			// FIXME FSCOM
			//var v2xServerUsd *SystemInformationBlockType21 = new(SystemInformationBlockType21)

			proInfoPc5[i-1] = Pc5ProvisioningInfoProInfoPc5{"dstLayer2Id", locationInfo, make([]Pc5NeighbourCellInfo, 0)}
		} // End of 'for' statement
	} else {
		err = errors.New("GetInfoPc5: Invalid parameter: " + params[0])
		proInfoPc5 = nil
	}

	log.Info("GetInfoPc5: proInfoPc5= ", proInfoPc5)
	return proInfoPc5, nil
}

func (tm *TrafficMgr) InitializeV2xMessageDistribution(v2xPoaList []string, poaNameList []string, ecgi_s []string, location_s [][]float32) (err error) {
	log.Debug(">>> InitializeV2xMessageDistribution: v2xPoaList: ", v2xPoaList)
	log.Debug(">>> InitializeV2xMessageDistribution: poaNameList: ", poaNameList)
	log.Debug(">>> InitializeV2xMessageDistribution: ecgi_s: ", ecgi_s)
	log.Debug(">>> InitializeV2xMessageDistribution: location_s: ", location_s)

	// Validate input
	if poaNameList == nil {
		err = errors.New("Missing POA Name List")
		return err
	}
	if ecgi_s == nil {
		err = errors.New("Missing ECGIs")
		return err
	}

	if len(ecgi_s) != 0 {
		cellName2CellIdMap = make(map[string]string, len(ecgi_s))
		cellId2CellNameMap = make(map[string]string, len(ecgi_s))
		v2xPoaListMap = make(map[string]string, len(ecgi_s))
		cellId2LocationMap = make(map[string][]float32, len(ecgi_s))
		for i := 0; i < len(ecgi_s); i++ {
			if ecgi_s[i] != "" {
				idx := sort.Search(len(tm.poa_list), func(j int) bool { return poaNameList[i] <= tm.poa_list[j] })
				if idx < len(tm.poa_list) {
					cellName2CellIdMap[poaNameList[i]] = ecgi_s[i]
					cellId2LocationMap[ecgi_s[i]] = location_s[i]
					cellId2CellNameMap[ecgi_s[i]] = poaNameList[i]
					// FIXME FSCOM Build the list of V2X compliant PoA
					res := func() bool {
						for _, s := range v2xPoaList {
							if s == poaNameList[i] {
								return true
							}
						} // End of 'for' statement
						return false
					}()
					if res {
						v2xPoaListMap[ecgi_s[i]] = poaNameList[i]
					}
				}
			}
		} // End of 'for' statement
		log.Info("InitializeV2xMessageDistribution: cellName2CellIdMap: ", cellName2CellIdMap)
		log.Info("InitializeV2xMessageDistribution: cellId2LocationMap: ", cellId2LocationMap)
		log.Info("InitializeV2xMessageDistribution: cellId2CellNameMap: ", cellId2CellNameMap)
		log.Info("InitializeV2xMessageDistribution: v2xPoaListMap: ", v2xPoaListMap)
	} else {
		log.Warn("InitializeV2xMessageDistribution: V2X message distribution ECGI list is empty")
	}

	return nil
}

/*
 * PublishMessageOnMessageBroker publish the provided V2X message (e.g. CA message) on brocker server (e.g. MQTT)
 * @param {string} msgContent The message to publish
 * @param {string} msgEncodeFormat contains the encoding format uses to encode the V2X raw message
 * @param {string} stdOrganization ontains the V2X standard which apply to the V2X message (e,g, ETSI for C-ITS)
 * @param {struct} msgType conatins the type of the V2X message (e.g. CA message)
 * @return {struct} nil on success, error otherwise
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Annex D (informative): State-of-the-art of using a Message Broker as V2X Message Distribution Server for exchanging non-session based V2X messages
 */
func (tm *TrafficMgr) PublishMessageOnMessageBroker(msgContent string, msgEncodeFormat string, stdOrganization string, msgType *int32) (err error) {
	if !brokerRunning {
		err = errors.New("Message broker mechanism not initialized")
		log.Error(err.Error())
		return err
	}
	return tm.message_broker.Send(tm, msgContent, msgEncodeFormat, stdOrganization, msgType)
}

/*
 * StartV2xMessageBrokerServer start the brocker server (e.g. MQTT)
 * @return {struct} nil on success, error otherwise
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Annex D (informative): State-of-the-art of using a Message Broker as V2X Message Distribution Server for exchanging non-session based V2X messages
 */
func (tm *TrafficMgr) StartV2xMessageBrokerServer() (err error) {
	if cellName2CellIdMap == nil || len(cellId2CellNameMap) == 0 {
		brokerRunning = false
		return
	}

	u, err := url.ParseRequestURI(tm.broker)
	if err != nil {
		err = errors.New("Failed to parse url " + tm.broker)
		log.Error(err.Error())
		return err
	}
	//log.Info("url:%v - scheme:%v - host:%v - Path:%v - Port:%s", u, u.Scheme, u.Hostname(), u.Path, u.Port())
	if u.Scheme == "mqtt" {
		tm.message_broker = &message_broker_mqtt{false, nil, nil, tm.v2x_notify}
	} else if u.Scheme == "amqp" {
		// TODO tm.message_broker = &message_broker_amqp{false}
		tm.message_broker = &message_broker_simu{false, map[int32][]byte{}, nil}
	} else {
		err = errors.New("Invalid url " + tm.broker)
		log.Error(err.Error())
		return err
	}

	err = tm.message_broker.Init(tm)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	err = tm.message_broker.Run(tm)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	brokerRunning = true

	return nil
}

/*
 * StopV2xMessageBrokerServer shutdown the brocker server (e.g. MQTT)
 * @return {struct} nil on success, error otherwise
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Annex D (informative): State-of-the-art of using a Message Broker as V2X Message Distribution Server for exchanging non-session based V2X messages
 */
func (tm *TrafficMgr) StopV2xMessageBrokerServer() {
	log.Info("StopV2xMessageBrokerServer: brokerRunning: ", brokerRunning)

	if brokerRunning {
		brokerRunning = false
		_ = tm.message_broker.Stop(tm)
	}
}
