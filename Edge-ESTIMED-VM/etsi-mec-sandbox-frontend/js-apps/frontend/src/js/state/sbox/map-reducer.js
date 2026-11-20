/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';

const initialState = {
  ueList: [],
  poaList: [],
  computeList: []
};

// CHANGE_MAP
const SBOX_CHANGE_MAP = 'SBOX_CHANGE_MAP';
export function sboxChangeMap(map) {
  return {
    type: SBOX_CHANGE_MAP,
    payload: map
  };
}

// SBOX_CHANGE_UE_LIST
const SBOX_CHANGE_UE_LIST = 'SBOX_CHANGE_UE_LIST';
export function sboxChangeMapUeList(ueList) {
  return {
    type: SBOX_CHANGE_UE_LIST,
    payload: ueList
  };
}

// SBOX_CHANGE_POA_LIST
const SBOX_CHANGE_POA_LIST = 'SBOX_CHANGE_POA_LIST';
export function sboxChangeMapPoaList(poaList) {
  return {
    type: SBOX_CHANGE_POA_LIST,
    payload: poaList
  };
}

// SBOX_CHANGE_COMPUTE_LIST
const SBOX_CHANGE_COMPUTE_LIST = 'SBOX_CHANGE_COMPUTE_LIST';
export function sboxChangeMapComputeList(computeList) {
  return {
    type: SBOX_CHANGE_COMPUTE_LIST,
    payload: computeList
  };
}

export function mapReducer(state = initialState, action) {
  switch (action.type) {
  case SBOX_CHANGE_MAP:
    return action.payload;
  case SBOX_CHANGE_UE_LIST:
    return updateObject(state, { ueList: action.payload });
  case SBOX_CHANGE_POA_LIST:
    return updateObject(state, { poaList: action.payload });
  case SBOX_CHANGE_COMPUTE_LIST:
    return updateObject(state, { computeList: action.payload });
  default:
    return state;
  }
}
