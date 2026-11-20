/*
 * Copyright (c) 2022-2025 ETSI.  All rights reserved.
 */

import _ from 'lodash';
import log from './logger';

import {
  // Element Fields
  FIELD_TYPE,
  FIELD_PARENT,
  FIELD_NAME,
  FIELD_NR_CELL_ID,
  FIELD_MAC_ID,
  FIELD_UE_MAC_ID,
  FIELD_CELL_ID,
  FIELD_CONNECTED,
  FIELD_WIRELESS,
  FIELD_WIRELESS_TYPE,
  FIELD_GEO_LOCATION,
  FIELD_GEO_RADIUS,
  FIELD_GEO_PATH,
  FIELD_GEO_EOP_MODE,
  FIELD_GEO_VELOCITY,
  FIELD_META_DISPLAY_MAP_COLOR,
  FIELD_META_DISPLAY_MAP_ICON,
  createElem,
  setElemFieldVal,
  FIELD_DATA_NETWORK_NAME,
  FIELD_EDGE_COMPUTE_PROVIDER,
  FIELD_DN_LADN
} from './elem-utils';

import {
  ELEMENT_TYPE_SCENARIO,
  ELEMENT_TYPE_OPERATOR,
  ELEMENT_TYPE_OPERATOR_CELL,
  ELEMENT_TYPE_ZONE,
  ELEMENT_TYPE_POA,
  ELEMENT_TYPE_POA_4G,
  ELEMENT_TYPE_POA_5G,
  ELEMENT_TYPE_POA_WIFI,
  ELEMENT_TYPE_DC,
  ELEMENT_TYPE_CN,
  ELEMENT_TYPE_EDGE,
  ELEMENT_TYPE_FOG,
  ELEMENT_TYPE_UE,
  ELEMENT_TYPE_MECSVC,
  ELEMENT_TYPE_UE_APP,
  ELEMENT_TYPE_EDGE_APP,
  ELEMENT_TYPE_CLOUD_APP,
  DOMAIN_TYPE_STR,
  DOMAIN_CELL_TYPE_STR,
  PUBLIC_DOMAIN_TYPE_STR,
  COMMON_ZONE_TYPE_STR,
  POA_TYPE_STR,
  POA_4G_TYPE_STR,
  POA_5G_TYPE_STR,
  POA_WIFI_TYPE_STR,
  DEFAULT_NL_TYPE_STR,
  UE_TYPE_STR,
  FOG_TYPE_STR,
  EDGE_TYPE_STR,
  CN_TYPE_STR,
  DC_TYPE_STR,
  MEC_SVC_TYPE_STR,
  UE_APP_TYPE_STR,
  EDGE_APP_TYPE_STR,
  CLOUD_APP_TYPE_STR,
  DEFAULT_VELOCITY_THRESHOLD
} from '../app-constants';

import {
  META_DISPLAY_MAP_COLOR,
  META_DISPLAY_MAP_ICON
} from './meta-keys';

// Parse scenario to populate map
export function parseScenario(scenario) {
  log.info(`parseScenario: Parsing scenario: ${scenario ? scenario.name : 'null'}`);
  if (!scenario) {
    return null;
  }

  var nodeList = new Array();

  // Populate node list
  addNode(scenario, nodeList);
  for (var i in scenario.deployment.domains) {
    var domain = scenario.deployment.domains[i];
    if (domain.name !== PUBLIC_DOMAIN_TYPE_STR) {
      addNode(domain, nodeList);
    }
    for (var j in domain.zones) {
      var zone = domain.zones[j];
      if (zone.name.indexOf(COMMON_ZONE_TYPE_STR) === -1) {
        addNode(zone, nodeList);
      }
      for (var k in zone.networkLocations) {
        var nl = zone.networkLocations[k];
        if (nl.name.indexOf(DEFAULT_NL_TYPE_STR) === -1) {
          addNode(nl, nodeList);
        }
        for (var l in nl.physicalLocations) {
          var pl = nl.physicalLocations[l];
          addNode(pl, nodeList);
          for (var m in pl.processes) {
            var proc = pl.processes[m];
            addNode(proc, nodeList);
          }
        }
      }
    }
  }

  // Create table from node list
  var table = {};
  table.entries = _.reduce(nodeList, (nodeMap, node) => {
    nodeMap[node.name] = getElementFromScenario(scenario, node.id);
    return nodeMap;
  }, {});

  return { table: table };
}

// Add node
export function addNode(element, nodes) {
  var n = {
    id: element.id ? element.id : element.name,
    name: element.name
  };
  nodes.push(n);
}

// Find the provided element in the scenario
export function getElementFromScenario(scenario, elementId) {
  // Create new element to be populated with scenario data
  var elem = createElem(elementId);

  // Check if scenario deployment is being requested
  if (scenario.name === elementId) {
    setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_SCENARIO);
    return elem;
  }

  // Loop through scenario until element is found
  for (var i in scenario.deployment.domains) {
    var domain = scenario.deployment.domains[i];
    if (domain.id === elementId) {

      switch (domain.type) {
      case DOMAIN_TYPE_STR:
        setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_OPERATOR);
        break;
      case DOMAIN_CELL_TYPE_STR:
        setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_OPERATOR_CELL);
        break;
      default:
        break;
      }

      setElemFieldVal(elem, FIELD_NAME, domain.name);
      setElemFieldVal(elem, FIELD_PARENT, scenario.name);
      return elem;
    }

    for (var j in domain.zones) {
      var zone = domain.zones[j];
      if (zone.id === elementId) {
        setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_ZONE);
        setElemFieldVal(elem, FIELD_NAME, zone.name);
        setElemFieldVal(elem, FIELD_PARENT, domain.type === PUBLIC_DOMAIN_TYPE_STR ? scenario.name : domain.name);
        if (zone.meta) {
          setElemFieldVal(elem, FIELD_META_DISPLAY_MAP_COLOR, zone.meta[META_DISPLAY_MAP_COLOR]);
        }
        return elem;
      }

      for (var k in zone.networkLocations) {
        var nl = zone.networkLocations[k];
        if (nl.id === elementId) {
          switch (nl.type) {
          case POA_TYPE_STR:
            setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_POA);
            break;
          case POA_4G_TYPE_STR:
            setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_POA_4G);
            break;
          case POA_5G_TYPE_STR:
            setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_POA_5G);
            break;
          case POA_WIFI_TYPE_STR:
            setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_POA_WIFI);
            break;
          default:
            break;
          }

          setElemFieldVal(elem, FIELD_NAME, nl.name);
          setElemFieldVal(elem, FIELD_PARENT, domain.type === PUBLIC_DOMAIN_TYPE_STR ?
            scenario.name : zone.type === COMMON_ZONE_TYPE_STR ? domain.name : zone.name);

          if (nl.poa4GConfig) {
            setElemFieldVal(elem, FIELD_CELL_ID, nl.poa4GConfig.cellId || '');
          }
          if (nl.poa5GConfig) {
            setElemFieldVal(elem, FIELD_NR_CELL_ID, nl.poa5GConfig.cellId || '');
          }
          if (nl.poaWifiConfig) {
            setElemFieldVal(elem, FIELD_MAC_ID, nl.poaWifiConfig.macId || '');
          }

          if (nl.geoData) {
            if (nl.geoData.location) {
              setElemFieldVal(elem, FIELD_GEO_LOCATION, JSON.stringify(nl.geoData.location.coordinates) || '');
            }
            setElemFieldVal(elem, FIELD_GEO_RADIUS, nl.geoData.radius || '');
          }
          if (nl.meta) {
            setElemFieldVal(elem, FIELD_META_DISPLAY_MAP_ICON, nl.meta[META_DISPLAY_MAP_ICON]);
          }
          return elem;
        }

        for (var l in nl.physicalLocations) {
          var pl = nl.physicalLocations[l];
          if (pl.id === elementId) {
            switch (pl.type) {
            case UE_TYPE_STR:
              setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_UE);
              break;
            case FOG_TYPE_STR:
              setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_FOG);
              break;
            case EDGE_TYPE_STR:
              setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_EDGE);
              break;
            case CN_TYPE_STR:
              setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_CN);
              break;
            case DC_TYPE_STR:
              setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_DC);
              break;
            default:
              break;
            }
            setElemFieldVal(elem, FIELD_NAME, pl.name);
            setElemFieldVal(elem, FIELD_PARENT, domain.type === PUBLIC_DOMAIN_TYPE_STR ? scenario.name :
              zone.type === COMMON_ZONE_TYPE_STR ? domain.name :
                nl.type === DEFAULT_NL_TYPE_STR ? zone.name : nl.name);
            setElemFieldVal(elem, FIELD_CONNECTED, pl.connected || false);
            setElemFieldVal(elem, FIELD_WIRELESS, pl.wireless || false);
            setElemFieldVal(elem, FIELD_WIRELESS_TYPE, pl.wirelessType || '');
            setElemFieldVal(elem, FIELD_UE_MAC_ID, pl.macId || '');

            if (pl.geoData) {
              if (pl.geoData.location) {
                setElemFieldVal(elem, FIELD_GEO_LOCATION, JSON.stringify(pl.geoData.location.coordinates) || '');
              }
              if (pl.geoData.path) {
                setElemFieldVal(elem, FIELD_GEO_PATH, JSON.stringify(pl.geoData.path.coordinates) || '');
              }
              setElemFieldVal(elem, FIELD_GEO_EOP_MODE, pl.geoData.eopMode || '');
              setElemFieldVal(elem, FIELD_GEO_VELOCITY, pl.geoData.velocity || '');
            }
            if (pl.dataNetwork) {
              if (pl.dataNetwork.dnn) {
                setElemFieldVal(elem, FIELD_DATA_NETWORK_NAME, pl.dataNetwork.dnn);
              }
              if (pl.dataNetwork.ecsp) {
                setElemFieldVal(elem, FIELD_EDGE_COMPUTE_PROVIDER, pl.dataNetwork.ecsp);
              }
              if (pl.dataNetwork.ladn) {
                setElemFieldVal(elem, FIELD_DN_LADN, pl.dataNetwork.ladn);
              }
            }
            if (pl.meta) {
              setElemFieldVal(elem, FIELD_META_DISPLAY_MAP_ICON, pl.meta[META_DISPLAY_MAP_ICON]);
            }
            return elem;
          }

          for (var m in pl.processes) {
            var process = pl.processes[m];
            if (process.id === elementId) {
              switch (process.type) {
              case MEC_SVC_TYPE_STR:
                setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_MECSVC);
                break;
              case UE_APP_TYPE_STR:
                setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_UE_APP);
                break;
              case EDGE_APP_TYPE_STR:
                setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_EDGE_APP);
                break;
              case CLOUD_APP_TYPE_STR:
                setElemFieldVal(elem, FIELD_TYPE, ELEMENT_TYPE_CLOUD_APP);
                break;
              default:
                break;
              }
              setElemFieldVal(elem, FIELD_PARENT, pl.name);
              setElemFieldVal(elem, FIELD_NAME, process.name);
              return elem;
            }
          }
        }
      }
    }
  }
}

export function getNetworkInfo(scenario) {
  if (!scenario) {
    return null;
  }

  // Get initial UE counts from scenario meta data
  if (scenario.deployment.userMeta) {
    var networkInfoUserMeta = scenario.deployment.userMeta['network-info'];
    if (networkInfoUserMeta) {
      var jsonObj = JSON.parse(networkInfoUserMeta);
      //process the object
      switch(jsonObj.type) {
      case 'local':
        return '../../img/' + jsonObj.path;
      case 'url':
        return jsonObj.path;
      default:
      }
    }
  }
  return null;
}

export function parseUes(scenario) {
  var totalHv = 0, totalLv = 0, totalZv = 0;
  var initHv = -1, initLv = -1, initZv = -1;
  var velocityThreshold = DEFAULT_VELOCITY_THRESHOLD;

  if (!scenario) {
    return null;
  }

  // Get initial UE counts from scenario meta data
  if (scenario.deployment.userMeta) {
    var mecSandboxUserMeta = scenario.deployment.userMeta['mec-sandbox'];
    if (mecSandboxUserMeta) {
      var jsonUeObj = JSON.parse(mecSandboxUserMeta);

      if (jsonUeObj.defaultStaticUeCount !== undefined && jsonUeObj.defaultStaticUeCount !== null) {
        initZv = parseInt(jsonUeObj.defaultStaticUeCount);
        if (isNaN(initZv)) {
          initZv = -1;
        }
      }
      if (jsonUeObj.defaultLowVelocityUeCount !== undefined && jsonUeObj.defaultLowVelocityUeCount !== null) {
        initLv = parseInt(jsonUeObj.defaultLowVelocityUeCount);
        if (isNaN(initLv)) {
          initLv = -1;
        }
      }
      if (jsonUeObj.defaultHighVelocityUeCount !== undefined && jsonUeObj.defaultHighVelocityUeCount !== null) {
        initHv = parseInt(jsonUeObj.defaultHighVelocityUeCount);
        if (isNaN(initHv)) {
          initHv = -1;
        }
      }
      if (jsonUeObj.highVelocitySpeedThreshold) {
        velocityThreshold = parseInt(jsonUeObj.highVelocitySpeedThreshold);
        if (isNaN(velocityThreshold)) {
          velocityThreshold = DEFAULT_VELOCITY_THRESHOLD;
        }
      }
    }
  }

  // Get all UEs from scenario
  var zvList = new Array();
  var lvList = new Array();
  var hvList = new Array();

  for (var i in scenario.deployment.domains) {
    var domain = scenario.deployment.domains[i];
    for (var j in domain.zones) {
      var zone = domain.zones[j];
      for (var k in zone.networkLocations) {
        var nl = zone.networkLocations[k];
        for (var l in nl.physicalLocations) {
          var pl = nl.physicalLocations[l];
          if ((pl.type === 'UE') && (pl.geoData)) {
            if (!pl.geoData.velocity || pl.geoData.velocity === 0) {
              totalZv++;
              zvList.push({name: pl.name, parentName: nl.name, geoData: pl.geoData, meta: pl.meta,
                connected: pl.connected, wireless: pl.wireless, wirelessType: pl.wirelessType, macId: pl.macId, processes: pl.processes});
            } else {
              if (pl.geoData.velocity >= velocityThreshold) {
                totalHv++;
                hvList.push({name: pl.name, parentName: nl.name, geoData: pl.geoData, meta: pl.meta,
                  connected: pl.connected, wireless: pl.wireless, wirelessType: pl.wirelessType, macId: pl.macId, processes: pl.processes});
              } else {
                totalLv++;
                lvList.push({name: pl.name, parentName: nl.name, geoData: pl.geoData, meta: pl.meta,
                  connected: pl.connected, wireless: pl.wireless, wirelessType: pl.wirelessType, macId: pl.macId, processes: pl.processes});
              }
            }
          }
        }
      }
    }
  }

  // Fallback unspecified initialization counts to total physical locations
  if (initHv === -1) { initHv = totalHv; }
  if (initLv === -1) { initLv = totalLv; }
  if (initZv === -1) { initZv = totalZv; }

  return {
    initHv: initHv,
    initLv: initLv,
    initZv: initZv,
    maxUeHv: totalHv,
    maxUeLv: totalLv,
    maxUeZv: totalZv,
    hvList: hvList,
    lvList: lvList,
    zvList: zvList
  };
}

export function parseMeps(scenario) {
  if (!scenario) {
    return [];
  }

  var meps = new Array();
  var nbMeps = 0;

  for (var i in scenario.deployment.domains) {
    var domain = scenario.deployment.domains[i];
    for (var j in domain.zones) {
      var zone = domain.zones[j];
      for (var k in zone.networkLocations) {
        var nl = zone.networkLocations[k];
        for (var l in nl.physicalLocations) {
          var pl = nl.physicalLocations[l];
          if (pl.type === 'EDGE') {
            //only check if an EDGE no for now, no other criteria to determine a MEP node
            meps.push(pl.name);
            nbMeps++;
          }
        }
      }
    }
  }
  if (nbMeps === 0) {
    meps.push('NOT AVAILABLE');
  }
  return meps;
}

export function parseEdgeApps(scenario) {
  if (!scenario) {
    return null;
  }

  var edgeApps = new Array();

  for (var i in scenario.deployment.domains) {
    var domain = scenario.deployment.domains[i];
    for (var j in domain.zones) {
      var zone = domain.zones[j];
      for (var k in zone.networkLocations) {
        var nl = zone.networkLocations[k];
        for (var l in nl.physicalLocations) {
          var pl = nl.physicalLocations[l];
          for (var p in pl.processes) {
            var proc = pl.processes[p];
            if (proc.type === 'EDGE-APP') {
              // MEC013
              if (proc.image.includes('meep-loc-serv')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Location%20Service%20REST%20API',
                  path: pl.name+'/location/v2',
                  pseudoName: 'Location (013) on ' + pl.name,
                  dropDownName: 'Location (013)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC012
              } else if (proc.image.includes('meep-rnis')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Radio%20Network%20Information%20Service%20REST%20API',
                  path: pl.name+'/rni/v2',
                  pseudoName: 'Radio Network Information (012) on ' + pl.name,
                  dropDownName: 'Radio Network Information (012)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC016
              } else if (proc.image.includes('meep-dai')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Device%20Application%20Interface%20REST%20API',
                  path: pl.name+'/dev_app/v1',
                  pseudoName: 'Device Application Interface (016) on ' + pl.name,
                  dropDownName: 'Device Application Interface (016)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC021
              } else if (proc.image.includes('meep-ams')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Application%20Mobility%20REST%20API',
                  path: pl.name+'/amsi/v1',
                  pseudoName: 'Application Mobility (021) on ' + pl.name,
                  dropDownName: 'Application Mobility (021)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC028
              } else if (proc.image.includes('meep-wais')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20WLAN%20Access%20Information%20Service%20REST%20API',
                  path: pl.name+'/wai/v2',
                  pseudoName: 'WLAN Access Information (028) on ' + pl.name,
                  dropDownName: 'WLAN Access Information (028)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC030
              } else if (proc.image.includes('meep-vis')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url:
                    'api/?urls.primaryName=' + pl.name + '%20-%20V2X%20Information%20Service%20REST%20API',
                  path: pl.name + '/vis/v2',
                  pseudoName: 'V2X Information (030) on ' + pl.name,
                  dropDownName: 'V2X Information (030)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC015
              } else if (proc.image.includes('meep-tm')) {
                // Bandwidth Management
                edgeApps.push({
                  id: proc.id,
                  name: proc.name+'-bwm',
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Bandwidth%20Management%20REST%20API',
                  path: pl.name+'/bwm/v1',
                  pseudoName: 'Bandwidth Management (015) on ' + pl.name,
                  dropDownName: 'Bandwidth Management (015)',
                  enabled: true,  // always running
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
                // Multi-access Traffic Steering
                edgeApps.push({
                  id: proc.id,
                  name: proc.name+'-mts',
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Multi-access%20Traffic%20Steering%20REST%20API',
                  path: pl.name+'/mts/v1',
                  pseudoName: 'Multi-access Traffic Steering (015) on ' + pl.name,
                  dropDownName: 'Multi-access Traffic Steering (015)',
                  enabled: true,  // always running
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC011
              } else if (proc.image.includes('meep-app-enablement')) {
                // App support
                edgeApps.push({
                  id: proc.id,
                  name: proc.name+'-app-support',
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20MEC%20Application%20Support%20API',
                  path: pl.name+'/mec_app_support/v2',
                  pseudoName: 'MEC Application Support (011) on ' + pl.name,
                  dropDownName: 'MEC Application Support (011)',
                  enabled: true,  // always running
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
                // Service Management
                edgeApps.push({
                  id: proc.id,
                  name: proc.name+'-service-mgmt',
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20MEC%20Service%20Management%20API',
                  path: pl.name+'/mec_service_mgmt/v1',
                  pseudoName: 'MEC Service Management (011) on ' + pl.name,
                  dropDownName: 'MEC Service Management (011)',
                  enabled: true,  // always running
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
                edgeApps.push({
                  id: proc.id,
                  name: proc.name+'-capif-mgmt',
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20MEC%20CAPIF%20Management%20API',
                  path: pl.name+'/service-apis/v1',
                  pseudoName: 'MEC-CAPIF Service Management (011) on ' + pl.name,
                  dropDownName: 'MEC-CAPIF Service Management (011)',
                  enabled: true,  // always running
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC016
              } else if (proc.image.includes('meep-dai')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Device%20Application%20Interface%20REST%20API',
                  path: pl.name+'/dev_app/v1',
                  pseudoName: 'Device Application Interface (016) on ' + pl.name,
                  dropDownName: 'Device Application Interface (016)',
                  enabled: true,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC033
              } else if (proc.image.includes('meep-iot')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url:
                    'api/?urls.primaryName=' + pl.name + '%20-%20IOT%20APIs%20REST%20API',
                  path: pl.name + '/iots/v1',
                  pseudoName: 'IOT API (033) on ' + pl.name,
                  dropDownName: 'IOT API (033)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC046
              } else if (proc.image.includes('meep-sss')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url:
                    'api/?urls.primaryName=' + pl.name + '%20-%20SENSORS%20SHARING%20APIs%20REST%20API',
                  path: pl.name + '/sens/v1',
                  pseudoName: 'SENSORS SHARING Service (046) on ' + pl.name,
                  dropDownName: 'SENSORS SHARING (046)',
                  enabled: false,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              // MEC040
              } else if (proc.image.includes('meep-federation')) {
                edgeApps.push({
                  id: proc.id,
                  name: proc.name,
                  instance: '',
                  img: proc.image,
                  mepName: pl.name,
                  env: proc.environment,
                  url: 'api/?urls.primaryName='+pl.name+'%20-%20Federation%20Service%20REST%20API',
                  path: pl.name+'/fed_enablement/v1',
                  pseudoName: 'MEC Federation APIs (040) on ' + pl.name,
                  dropDownName: 'MEC Federation APIs (040)',
                  enabled: true,
                  enableInProgressCount: -1,
                  disableInProgressCount: -1
                });
              }
            }
          }
        }
      }
    }
  }
  return edgeApps;
}
