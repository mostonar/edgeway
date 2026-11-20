/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

// Network Element Fields
export const FIELD_TYPE = 'elementType';
export const FIELD_PARENT = 'parent';
export const FIELD_NAME = 'name';
export const FIELD_CELL_ID = 'cellId';
export const FIELD_NR_CELL_ID = 'nrCellId';
export const FIELD_MAC_ID = 'macId';
export const FIELD_UE_MAC_ID = 'ueMacId';
export const FIELD_CONNECTED = 'connected';
export const FIELD_WIRELESS = 'wireless';
export const FIELD_WIRELESS_TYPE = 'wirelessType';
export const FIELD_GEO_LOCATION = 'location';
export const FIELD_GEO_RADIUS = 'radius';
export const FIELD_GEO_PATH = 'path';
export const FIELD_GEO_EOP_MODE = 'eopMode';
export const FIELD_GEO_VELOCITY = 'velocity';
export const FIELD_META_DISPLAY_MAP_COLOR = 'metaDisplayMapColor';
export const FIELD_META_DISPLAY_MAP_ICON = 'metaDisplayMapIcon';
export const FIELD_DATA_NETWORK_NAME = 'dataNetworkName';
export const FIELD_EDGE_COMPUTE_PROVIDER = 'edgeServiceProvider';
export const FIELD_DN_LADN = 'ladn';

export const getElemFieldVal = (elem, field) => {
  return (elem && elem[field]) ? elem[field].val : null;
};

export const setElemFieldVal = (elem, field, val) => {
  if (elem) {
    elem[field] = { val: val, err: null };
  }
};

export const getElemFieldErr = (elem, field) => {
  return (elem && elem[field]) ? elem[field].err : null;
};

export const setElemFieldErr = (elem, field, err) => {
  if (elem) {
    elem[field].err = err;
  }
};

export const createElem = name => {
  var elem = {};
  setElemFieldVal(elem, FIELD_TYPE, '');
  setElemFieldVal(elem, FIELD_PARENT, '');
  setElemFieldVal(elem, FIELD_NAME, name);
  setElemFieldVal(elem, FIELD_CELL_ID, '');
  setElemFieldVal(elem, FIELD_NR_CELL_ID, '');
  setElemFieldVal(elem, FIELD_MAC_ID, '');
  setElemFieldVal(elem, FIELD_UE_MAC_ID, '');
  setElemFieldVal(elem, FIELD_CONNECTED, true);
  setElemFieldVal(elem, FIELD_WIRELESS, false);
  setElemFieldVal(elem, FIELD_WIRELESS_TYPE, '');
  setElemFieldVal(elem, FIELD_GEO_LOCATION, '');
  setElemFieldVal(elem, FIELD_GEO_RADIUS, '');
  setElemFieldVal(elem, FIELD_GEO_PATH, '');
  setElemFieldVal(elem, FIELD_GEO_EOP_MODE, '');
  setElemFieldVal(elem, FIELD_GEO_VELOCITY, '');
  setElemFieldVal(elem, FIELD_META_DISPLAY_MAP_COLOR, '');
  setElemFieldVal(elem, FIELD_META_DISPLAY_MAP_ICON, '');
  return elem;
};
