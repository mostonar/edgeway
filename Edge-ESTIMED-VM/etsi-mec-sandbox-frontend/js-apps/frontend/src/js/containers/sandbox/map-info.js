/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component, createRef } from 'react';
import ReactDOM from 'react-dom';
import L from 'leaflet';
import 'mapbox-gl';
import 'mapbox-gl-leaflet';
import tinycolor from 'tinycolor2';
import _ from 'lodash';

import {
  updateObject
} from '../../util/object-util';

import {
  FIELD_TYPE,
  FIELD_PARENT,
  FIELD_CELL_ID,
  FIELD_NR_CELL_ID,
  FIELD_MAC_ID,
  FIELD_UE_MAC_ID,
  FIELD_CONNECTED,
  FIELD_WIRELESS_TYPE,
  FIELD_META_DISPLAY_MAP_COLOR,
  FIELD_META_DISPLAY_MAP_ICON,
  getElemFieldVal,
  FIELD_DATA_NETWORK_NAME,
  FIELD_EDGE_COMPUTE_PROVIDER,
  FIELD_DN_LADN
} from '../../util/elem-utils';

import {
  uiSandboxChangeMapCfg
} from '../../state/ui';

import {
  HOST_PATH,
  ELEMENT_TYPE_EDGE,
  ELEMENT_TYPE_FOG,
  ELEMENT_TYPE_POA_4G,
  ELEMENT_TYPE_POA_5G,
  ELEMENT_TYPE_POA_WIFI
} from '../../app-constants';

import 'leaflet/dist/images/marker-shadow.png';

const ZONE_COLOR_LIST = [
  'blueviolet',
  'darkorange',
  'darkred',
  'limegreen',
  'blue',
  'purple',
  'gold',
  'darkturquoise'
];
const DISCONNECTED_COLOR = 'red';

const LOCATION_PRECISION = 6;

const UE_ICON = 'ion-iphone';
const UE_COLOR_DEFAULT = '#00ccff';
const UE_PATH_COLOR = '#008fb3';
const UE_OPACITY = 1.0;
const UE_PATH_OPACITY = 0.5;

const POA_ICON = 'ion-connection-bars';
const POA_ICON_WIFI = 'ion-wifi';
const POA_COLOR_DEFAULT = '#696969';
const POA_OPACITY = 1.0;
const POA_RANGE_OPACITY = 0.05;

const COMPUTE_ICON = 'ion-android-cloud';
const COMPUTE_COLOR_DEFAULT = '#0a50f2';
const COMPUTE_OPACITY = 1.0;

const DEFAULT_MAP_STYLE = 'Positron';
const DEFAULT_MAP_LATITUDE = 43.737368;
const DEFAULT_MAP_LONGITUDE = 7.427874;
const DEFAULT_MAP_ZOOM = 14;

class MapInfo extends Component {
  constructor(props) {
    super(props);
    this.state = {};
    this.thisRef = createRef();
    this.configRef = createRef();
    this.rendering = false;
    this.zoneColorMap = {};
  }

  componentDidMount() {
    this.createMap();
  }

  componentWillUnmount() {
    this.destroyMap();
  }

  componentDidUpdate(prevProps) {
    if (prevProps.sandbox !== this.props.sandbox) {
      this.destroyMap();
      this.createMap();
      this.updateMarkers();
    }
  }

  shouldComponentUpdate() {
    // Map size update
    let width = this.thisRef.current.offsetWidth;
    let height = this.thisRef.current.offsetHeight;
    if ((width && this.width !== width) || (height && this.height !== height)) {
      this.width = width;
      this.height = height;
      // console.log('Map view resized to: ' + width + 'x' + height);
      this.map.invalidateSize();
    }
    return true;
  }

  getTable() {
    return this.props.table;
  }

  updateCfg(cfg) {
    this.props.changeMapCfg(updateObject(this.props.mapCfg, cfg));
  }

  createMap() {
    // Get stored configuration
    var cfg = this.props.mapCfg;
    var lat = (cfg && cfg.center) ? cfg.center.lat : DEFAULT_MAP_LATITUDE;
    var lng = (cfg && cfg.center) ? cfg.center.lng : DEFAULT_MAP_LONGITUDE;
    var zoom = (cfg && cfg.zoom) ? cfg.zoom : DEFAULT_MAP_ZOOM;
    var baselayerName = (cfg && cfg.baselayerName) ? cfg.baselayerName : DEFAULT_MAP_STYLE;

    // Map bounds
    const corner1 = L.latLng(43.68, 7.35);
    const corner2 = L.latLng(43.80, 7.55);
    const bounds = L.latLngBounds(corner1, corner2);

    // Create Map instance
    var domNode = ReactDOM.findDOMNode(this);
    this.map = L.map(domNode, {
      center: [lat,lng],
      zoom: zoom,
      minZoom: 14,
      maxZoom: 20,
      drawControl: true,
      maxBounds: bounds,
      maxBoundsViscosity: 1.0
    });
    this.map.attributionControl.addAttribution('<a href="https://www.maptiler.com/copyright/?_ga=2.45788834.742970109.1593090041-1523068243.1593090041" target="_blank">© MapTiler</a>');
    this.map.attributionControl.addAttribution('<a href="https://www.openstreetmap.org/copyright" target="_blank">© OpenStreetMap contributors</a>');

    // Create GL Baselayers
    var positronBaselayer = L.mapboxGL({style: HOST_PATH + '/map/styles/positron/style.json'});
    var darkBaselayer = L.mapboxGL({style: HOST_PATH + '/map/styles/dark-matter/style.json'});
    var klokBaselayer = L.mapboxGL({style: HOST_PATH + '/map/styles/klokantech-basic/style.json'});
    var osmBaselayer = L.mapboxGL({style: HOST_PATH + '/map/styles/osm-bright/style.json'});
    var baselayers = {
      'Positron': positronBaselayer,
      'Black Matter': darkBaselayer,
      'Klokantech': klokBaselayer,
      'OSM Bright': osmBaselayer
    };

    // Create Layer Group Overlays
    this.ueOverlay = L.layerGroup();
    this.uePathOverlay = L.layerGroup();
    this.poaOverlay = L.layerGroup();
    this.poaRangeOverlay = L.layerGroup();
    this.computeOverlay = L.layerGroup();
    var overlays = {
      'terminal': this.ueOverlay,
      'terminal-path': this.uePathOverlay,
      'poa': this.poaOverlay,
      'poa-coverage': this.poaRangeOverlay,
      'compute': this.computeOverlay
    };

    // Create Layer Controller
    this.layerCtrl = L.control.layers(baselayers, overlays);

    // Create popup
    this.popup = L.popup();

    // Initialize map & layers
    this.layerCtrl.addTo(this.map);
    this.ueOverlay.addTo(this.map);
    this.uePathOverlay.addTo(this.map);
    this.poaOverlay.addTo(this.map);
    this.poaRangeOverlay.addTo(this.map);
    this.computeOverlay.addTo(this.map);

    // Set default base layer
    var baselayer = baselayers[baselayerName] ? baselayers[baselayerName] : positronBaselayer;
    baselayer.addTo(this.map);

    // Handlers
    var _this = this;
    this.map.on('zoomend', function() {_this.setZoom(this);});
    this.map.on('moveend', function() {_this.setCenter(this);});
    this.map.on('baselayerchange', function(e) {_this.setBaseLayer(e);});

    // Add asset markers
    this.updateMarkers();
  }

  destroyMap() {
    if (this.map) {
      this.map.remove();
    }
  }

  setZoom(map) {
    if (map && !this.rendering) {
      this.updateCfg({zoom: map.getZoom()});
    }
  }

  setCenter(map) {
    if (map && !this.rendering) {
      this.updateCfg({center: map.getCenter()});
    }
  }

  setBaseLayer(event) {
    this.updateCfg({baselayerName: event.name});
  }

  getUePoa(ue) {
    var poa = null;
    var table = this.getTable();
    if (table && table.entries) {
      poa = getElemFieldVal(table.entries[ue], FIELD_PARENT);
    }
    return poa;
  }

  getUeZone(ue) {
    var zone = null;
    var table = this.getTable();
    if (table && table.entries) {
      var poa = getElemFieldVal(table.entries[ue], FIELD_PARENT);
      zone = poa ? this.getPoaZone(poa) : null;
    }
    return zone;
  }

  getPoaZone(poa) {
    var zone = null;
    var table = this.getTable();
    if (table && table.entries) {
      zone = getElemFieldVal(table.entries[poa], FIELD_PARENT);
    }
    return zone;
  }

  getComputeZone(compute) {
    var zone = null;
    var table = this.getTable();
    if (table && table.entries) {
      var computeType = getElemFieldVal(table.entries[compute], FIELD_TYPE);
      var parent = getElemFieldVal(table.entries[compute], FIELD_PARENT);
      if (computeType === ELEMENT_TYPE_EDGE) {
        zone = parent;
      } else if (computeType === ELEMENT_TYPE_FOG) {
        zone = parent ? this.getPoaZone(parent) : null;
      }
    }
    return zone;
  }

  // Get Colors
  getZoneColor(zone) {
    var color = null;
    var table = this.getTable();
    if (zone && table && table.entries) {
      // Get zone color from meta
      color = getElemFieldVal(table.entries[zone], FIELD_META_DISPLAY_MAP_COLOR);
      if (!color) {
        // Get zone color from zone color map
        color = this.zoneColorMap[zone];
        if (!color) {
          // Get a new color for this zone
          color = this.zoneColorMap[zone] = ZONE_COLOR_LIST[Object.keys(this.zoneColorMap).length % ZONE_COLOR_LIST.length];
          // // Generate a random color for this zone
          // color = this.zoneColorMap[zone] = tinycolor.random().toHexString();
        }
      }
    }
    return color;
  }

  getUeColor(ue) {
    // var color = this.getZoneColor(this.getUeZone(ue));
    var color = undefined;
    if (!this.isConnected(ue)) {
      color = DISCONNECTED_COLOR;
    }
    return color ? color : UE_COLOR_DEFAULT;
  }

  getPoaColor(poa) {
    var color = this.getZoneColor(this.getPoaZone(poa));
    return color ? color : POA_COLOR_DEFAULT;
  }

  getComputeColor(compute) {
    if (!this.isConnected(compute)) {
      return DISCONNECTED_COLOR;
    }
    return COMPUTE_COLOR_DEFAULT;
  }

  // Get connected status
  isConnected(name) {
    var connected = false;
    var table = this.getTable();
    if (table && table.entries) {
      connected = getElemFieldVal(table.entries[name], FIELD_CONNECTED);
    }
    return connected;
  }

  // Get wireless type Priority
  getWirelessTypePrio(name) {
    var wirelessTypePrio = '';
    var table = this.getTable();
    if (table && table.entries) {
      wirelessTypePrio = getElemFieldVal(table.entries[name], FIELD_WIRELESS_TYPE);
    }
    return wirelessTypePrio;
  }

  // Set Icons
  setUeIcon(iconDiv, ue) {
    var table = this.getTable();
    if (table && table.entries) {
      var metaIcon = getElemFieldVal(table.entries[ue], FIELD_META_DISPLAY_MAP_ICON);
      var icon = metaIcon ? metaIcon : UE_ICON;
      iconDiv.className = 'custom-marker-icon ion ' + icon;
      iconDiv.innerHTML = '';
    }
  }

  setPoaIcon(iconDiv, iconTextDiv, poa) {
    var table = this.getTable();
    if (table && table.entries) {
      var poaType = getElemFieldVal(table.entries[poa], FIELD_TYPE);
      var metaIcon = getElemFieldVal(table.entries[poa], FIELD_META_DISPLAY_MAP_ICON);
      var icon = metaIcon ? metaIcon : (poaType === ELEMENT_TYPE_POA_WIFI) ? POA_ICON_WIFI : POA_ICON;
      iconDiv.className = 'custom-marker-icon ion ' + icon;
      iconDiv.innerHTML = '';

      var innerHTML = '';
      if (!metaIcon) {
        if (poaType === ELEMENT_TYPE_POA_4G) {
          innerHTML = '4G';
        } else {
          if (poaType === ELEMENT_TYPE_POA_5G) {
            innerHTML = '5G';
          }
        }
      }
      iconTextDiv.innerHTML = innerHTML;
    }
  }

  setComputeIcon(iconDiv, compute) {
    var table = this.getTable();
    if (table && table.entries) {
      var metaIcon = getElemFieldVal(table.entries[compute], FIELD_META_DISPLAY_MAP_ICON);
      var icon = metaIcon ? metaIcon : COMPUTE_ICON;
      iconDiv.className = 'custom-marker-icon ion ' + icon;
      iconDiv.innerHTML = '';
    }
  }

  // Set styles
  setUeMarkerStyle(marker) {
    if (marker._icon) {
      // Set marker color
      var color = tinycolor(this.getUeColor(marker.options.meep.ue.id));
      var markerStyle = marker._icon.querySelector('.custom-marker-pin').style;
      markerStyle['background'] = color;
      markerStyle['border-color'] = color.darken(10);

      // Set marker icon
      var iconDiv = marker._icon.querySelector('.custom-marker-icon');
      this.setUeIcon(iconDiv, marker.options.meep.ue.id);
    }
  }

  setPoaMarkerStyle(marker) {
    if (marker._icon) {
      // Set marker color
      var color = tinycolor(this.getPoaColor(marker.options.meep.poa.id));
      var markerStyle = marker._icon.querySelector('.custom-marker-pin').style;
      markerStyle['background'] = color;
      markerStyle['border-color'] = color.darken(10);

      // Set POA range color
      marker.options.meep.poa.range.setStyle({color: color});

      // Set marker icon
      var iconDiv = marker._icon.querySelector('.custom-marker-icon');
      var iconTextDiv = marker._icon.querySelector('.custom-marker-icon-text');
      this.setPoaIcon(iconDiv, iconTextDiv, marker.options.meep.poa.id);
    }
  }

  setComputeMarkerStyle(marker) {
    if (marker._icon) {
      // Set marker color
      var color = tinycolor(this.getComputeColor(marker.options.meep.compute.id));
      var markerStyle = marker._icon.querySelector('.custom-marker-pin').style;
      markerStyle['background'] = color;
      markerStyle['border-color'] = color.darken(10);

      // Set marker icon
      var iconDiv = marker._icon.querySelector('.custom-marker-icon');
      this.setComputeIcon(iconDiv, marker.options.meep.compute.id);
    }
  }

  getLocationStr(latlng) {
    return '[' + latlng.lat.toFixed(LOCATION_PRECISION) + ', ' + latlng.lng.toFixed(LOCATION_PRECISION) + ']';
  }

  // UE Marker Event Handler
  updateUePopup(marker) {
    var table = this.getTable();
    if (marker && table && table.entries) {
      var latlng = marker.getLatLng();
      var hasPath = (marker.options.meep.ue.path) ? true : false;
      var msg = '<b>id: ' + marker.options.meep.ue.id + '</b><br>';
      var ownMac = getElemFieldVal(table.entries[marker.options.meep.ue.id], FIELD_UE_MAC_ID);
      if (ownMac !== '') {
        msg += 'mac: ' + ownMac + '<br>';
      }
      msg += 'velocity: ' + (hasPath ? marker.options.meep.ue.velocity : '0') + ' m/s<br>';

      if (this.isConnected(marker.options.meep.ue.id)) {
        var poa = this.getUePoa(marker.options.meep.ue.id);
        var poaType = getElemFieldVal(table.entries[poa], FIELD_TYPE);
        msg += 'poa: ' + poa + '<br>';
        switch(poaType) {
        case ELEMENT_TYPE_POA_4G:
          msg += 'cell: ' + getElemFieldVal(table.entries[poa], FIELD_CELL_ID) + '<br>';
          break;
        case ELEMENT_TYPE_POA_5G:
          msg += 'cell: ' + getElemFieldVal(table.entries[poa], FIELD_NR_CELL_ID) + '<br>';
          break;
        case ELEMENT_TYPE_POA_WIFI:
          msg += 'poa mac: ' + getElemFieldVal(table.entries[poa], FIELD_MAC_ID) + '<br>';
          break;
        default:
          break;
        }

        msg += 'zone: ' + this.getUeZone(marker.options.meep.ue.id) + '<br>';
      } else {
        msg += 'state: <b style="color:red;">DISCONNECTED</b><br>';
      }

      msg += 'wireless: ' + (this.getWirelessTypePrio(marker.options.meep.ue.id) || 'wifi,5g,4g,other') + '<br>';
      msg += 'location: ' + this.getLocationStr(latlng);
      marker.getPopup().setContent(msg);
    }
  }

  // POA Marker Event Handler
  updatePoaPopup(marker) {
    var table = this.getTable();
    if (marker && table && table.entries) {
      var latlng = marker.getLatLng();
      var poaType = getElemFieldVal(table.entries[marker.options.meep.poa.id], FIELD_TYPE);
      var msg = '<b>id: ' + marker.options.meep.poa.id + '</b><br>';
      msg += 'radius: ' + marker.options.meep.poa.range.options.radius + ' m<br>';
      switch(poaType) {
      case ELEMENT_TYPE_POA_4G:
        msg += 'cell: ' + getElemFieldVal(table.entries[marker.options.meep.poa.id], FIELD_CELL_ID) + '<br>';
        break;
      case ELEMENT_TYPE_POA_5G:
        msg += 'cell: ' + getElemFieldVal(table.entries[marker.options.meep.poa.id], FIELD_NR_CELL_ID) + '<br>';
        break;
      case ELEMENT_TYPE_POA_WIFI:
        msg += 'mac: ' + getElemFieldVal(table.entries[marker.options.meep.poa.id], FIELD_MAC_ID) + '<br>';
        break;
      default:
        break;
      }

      msg += 'zone: ' + this.getPoaZone(marker.options.meep.poa.id) + '<br>';
      msg += 'location: ' + this.getLocationStr(latlng);
      marker.getPopup().setContent(msg);
    }
  }



  // UE Marker Event Handler
  updateComputePopup(marker) {
    var table = this.getTable();
    if (marker && table && table.entries) {
      const networkName = getElemFieldVal(table.entries[marker.options.meep.compute.id], FIELD_DATA_NETWORK_NAME);
      const edgeProvider = getElemFieldVal(table.entries[marker.options.meep.compute.id], FIELD_EDGE_COMPUTE_PROVIDER);
      const ladn = getElemFieldVal(table.entries[marker.options.meep.compute.id], FIELD_DN_LADN);
      var appInstanceTable = this.props.appInstanceTable;
      var latlng = marker.getLatLng();
      // Parse mec application state on current popup
      var appInstances = [];
      for (var i = 0; i < appInstanceTable.length ; i++) {
        if (appInstanceTable[i].nodeName === marker.options.meep.compute.id) {
          appInstances.push(appInstanceTable[i]);
        }
      }
      // Sort parsed array of mec app
      var sortedAppInstances = _.sortBy(appInstances, ['name']);
      // Modify render message
      var msg = '<b>id: ' + marker.options.meep.compute.id + '</b><br>';
      if (edgeProvider) {
        msg += 'service-provider: ' + edgeProvider + '<br>';
      }
      if (networkName) {
        msg += 'data-network: ' + networkName;
        if (ladn) {
          msg += ' (LADN)';
        }
        msg += '<br>';
      }
      msg += 'applications: <br>';
      if (appInstances) {
        sortedAppInstances.forEach(elem => {
          msg += '<li>' + elem.name + ' ' + '(id: ' + elem.id.substring(0,8) + '...)' + '<br>';
        });
      }
      msg += 'geo-location: ' + this.getLocationStr(latlng);
      marker.getPopup().setContent(msg);
    }
  }


  setUeMarker(ue) {
    var latlng = L.latLng(L.GeoJSON.coordsToLatLng(ue.location.coordinates));
    var pathLatLngs = ue.path ? L.GeoJSON.coordsToLatLngs(ue.path.coordinates) : null;

    // Find existing UE marker
    var existingMarker;
    this.ueOverlay.eachLayer((marker) => {
      if (marker.options.meep.ue.id === ue.assetName){
        existingMarker = marker;
        return;
      }
    });

    if (existingMarker === undefined) {
      // Create path, if any
      var p = !pathLatLngs ? null : L.polyline(pathLatLngs, {
        meep: {
          path: {
            id: ue.assetName
          }
        },
        color: UE_PATH_COLOR,
        opacity: UE_PATH_OPACITY
      });

      var markerIcon = L.divIcon({
        className: '',
        html: '<div class="custom-marker-pin"></div><div class="custom-marker-icon"></div>',
        iconSize: [30, 42],
        iconAnchor: [15, 42],
        popupAnchor: [0, -36]
      });

      // Create new UE marker
      var m = L.marker(latlng, {
        meep: {
          ue: {
            id: ue.assetName,
            path: p,
            eopMode: ue.eopMode,
            velocity: ue.velocity,
            connected: true
          }
        },
        icon: markerIcon,
        opacity: UE_OPACITY,
        draggable: false
      });
      m.bindTooltip(ue.assetName).openTooltip();
      m.bindPopup('').openPopup();

      // Handlers
      var _this = this;
      m.on('add', (e) => _this.setUeMarkerStyle(e.target));
      m.on('popupopen', (e) => _this.updateUePopup(e.target));

      // Add to map overlay
      m.addTo(this.ueOverlay);
      if (p) {
        p.addTo(this.uePathOverlay);
        // p.setOpacity(UE_PATH_OPACITY);
      }

    } else {
      // Update UE position, , path, mode & velocity
      existingMarker.setLatLng(latlng);
      existingMarker.options.meep.ue.eopMode = ue.eopMode;
      existingMarker.options.meep.ue.velocity = ue.velocity;

      // Update, create or remove path
      if (pathLatLngs) {
        if (existingMarker.options.meep.ue.path) {
          existingMarker.options.meep.ue.path.setLatLngs(pathLatLngs);
        } else {
          var path = L.polyline(pathLatLngs, {
            meep: {
              path: {
                id: ue.assetName
              }
            },
            color: UE_PATH_COLOR,
            opacity: UE_PATH_OPACITY
          });
          existingMarker.options.meep.ue.path = path;
          path.addTo(this.uePathOverlay);
        }
      } else {
        if (existingMarker.options.meep.ue.path) {
          existingMarker.options.meep.ue.path.removeFrom(this.uePathOverlay);
          existingMarker.options.meep.ue.path = null;
        }
      }

      // Refresh marker style if necessary
      // var connected = this.isConnected(ue.assetName);
      // if (existingMarker.options.meep.ue.connected !== connected) {
      this.setUeMarkerStyle(existingMarker);
      //   existingMarker.options.meep.ue.connected = connected;
      // }

      // Refresh marker position
      this.updateUePopup(existingMarker);
    }
  }

  setPoaMarker(poa) {
    var latlng = L.latLng(L.GeoJSON.coordsToLatLng(poa.location.coordinates));

    // Find existing POA marker
    var existingMarker;
    this.poaOverlay.eachLayer((marker) => {
      if (marker.options.meep.poa.id === poa.assetName){
        existingMarker = marker;
        return;
      }
    });

    if (existingMarker === undefined) {
      // Create new POA marker & circle
      var c = L.circle(latlng, {
        meep: {
          range: {
            id: poa.assetName
          }
        },
        color: this.getPoaColor(poa.assetName),
        radius: poa.radius || 0,
        opacity: POA_RANGE_OPACITY
      });

      var markerIcon = L.divIcon({
        className: '',
        html: '<div class="custom-marker-pin"></div><div class="custom-marker-icon"></div><div class="custom-marker-icon-text"></div>',
        iconSize: [30, 42],
        iconAnchor: [15, 42],
        popupAnchor: [0, -36]
      });

      // Create new marker
      var m = L.marker(latlng, {
        meep: {
          poa: {
            id: poa.assetName,
            range: c
          }
        },
        icon: markerIcon,
        opacity: POA_OPACITY,
        draggable: false
      });
      m.bindTooltip(poa.assetName).openTooltip();
      m.bindPopup('').openPopup();

      // Handlers
      var _this = this;
      m.on('add', (e) => _this.setPoaMarkerStyle(e.target));
      m.on('popupopen', (e) => _this.updatePoaPopup(e.target));

      // Add to map overlay
      m.addTo(this.poaOverlay);
      c.addTo(this.poaRangeOverlay);

    } else {
      // Update POA position & range
      existingMarker.setLatLng(latlng);
      existingMarker.options.meep.poa.range.setLatLng(latlng);
      if (Number.isInteger(poa.radius) && poa.radius >= 0) {
        existingMarker.options.meep.poa.range.setRadius(poa.radius);
      }

      // Refresh marker style & position
      this.setPoaMarkerStyle(existingMarker);
      this.updatePoaPopup(existingMarker);
    }
  }

  setComputeMarker(compute) {
    var latlng = L.latLng(L.GeoJSON.coordsToLatLng(compute.location.coordinates));

    // Find existing COMPUTE marker
    var existingMarker;
    this.computeOverlay.eachLayer((marker) => {
      if (marker.options.meep.compute.id === compute.assetName){
        existingMarker = marker;
        return;
      }
    });

    if (existingMarker === undefined) {
      // Create new marker
      var markerIcon = L.divIcon({
        className: '',
        html: '<div class="custom-marker-pin"></div><div class="custom-marker-icon"></div>',
        iconSize: [30, 42],
        iconAnchor: [15, 42],
        popupAnchor: [0, -36]
      });

      // Creating new COMPUTE marker
      var m = L.marker(latlng, {
        meep: {
          compute: {
            id: compute.assetName,
            connected: true
          }
        },
        icon: markerIcon,
        opacity: COMPUTE_OPACITY,
        draggable: false
      });
      m.bindTooltip(compute.assetName).openTooltip();
      m.bindPopup('').openPopup();

      // Handlers
      var _this = this;
      m.on('add', (e) => _this.setComputeMarkerStyle(e.target));
      m.on('popupopen', (e) => _this.updateComputePopup(e.target));

      // Add to map overlay
      m.addTo(this.computeOverlay);

    } else {
      // Update COMPUTE position
      existingMarker.setLatLng(latlng);

      // Refresh marker style if necessary
      // var connected = this.isConnected(compute.assetName);
      // if (existingMarker.options.meep.compute.connected !== connected) {
      this.setComputeMarkerStyle(existingMarker);
      //   existingMarker.options.meep.compute.connected = connected;
      // }

      // Refresh marker position
      this.updateComputePopup(existingMarker);
    }
  }

  updateMarkers() {
    if (!this.map) {
      return;
    }

    // Get copy of map data 
    var map = this.props.map;
    if (!map) {
      return;
    }

    // Set COMPUTE markers
    var computeMap = {};
    if (map.computeList) {
      for (let i = 0; i < map.computeList.length; i++) {
        let compute = map.computeList[i];
        this.setComputeMarker(compute);
        computeMap[compute.assetName] = true;
      }
    }

    // Remove old COMPUTE markers
    this.computeOverlay.eachLayer((marker) => {
      if (!computeMap[marker.options.meep.compute.id]) {
        marker.removeFrom(this.computeOverlay);
      }
    });

    // Set POA markers
    var poaMap = {};
    if (map.poaList) {
      for (let i = 0; i < map.poaList.length; i++) {
        let poa = map.poaList[i];
        this.setPoaMarker(poa);
        poaMap[poa.assetName] = true;
      }
    }

    // Remove old POA markers
    this.poaOverlay.eachLayer((marker) => {
      if (!poaMap[marker.options.meep.poa.id]) {
        marker.options.meep.poa.range.removeFrom(this.poaRangeOverlay);
        marker.removeFrom(this.poaOverlay);
      }
    });

    // Set UE markers
    var ueMap = {};
    if (map.ueList) {
      for (let i = 0; i < map.ueList.length; i++) {
        let ue = map.ueList[i];
        this.setUeMarker(ue);
        ueMap[ue.assetName] = true;
      }
    }

    // Remove old UE markers
    this.ueOverlay.eachLayer((marker) => {
      if (!ueMap[marker.options.meep.ue.id]) {
        if (marker.options.meep.ue.path) {
          marker.options.meep.ue.path.removeFrom(this.uePathOverlay);
        }
        marker.removeFrom(this.ueOverlay);
      }
    });
  }

  render() {
    this.rendering = true;
    this.updateMarkers();
    this.rendering = false;
    return (
      <div ref={this.thisRef} style={{ height: '100%' }}>
        Map Component
      </div>
    );
  }
}

const mapStateToProps = state => {
  return {
    // UI
    sandbox: state.ui.sandbox,
    mapCfg: state.ui.mapCfg,
    // SBOX
    map: state.sbox.map,
    table: state.sbox.table,
    // EDGE APPS
    appInstanceTable: state.sbox.appInstanceTable.data
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeMapCfg: cfg => dispatch(uiSandboxChangeMapCfg(cfg))
  };
};

const ConnectedMapInfo = connect(
  mapStateToProps,
  mapDispatchToProps
)(MapInfo);

export default ConnectedMapInfo;
