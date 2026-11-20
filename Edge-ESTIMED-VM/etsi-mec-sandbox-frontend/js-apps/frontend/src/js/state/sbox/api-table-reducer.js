/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';

const initialState = {
  data: null
};

const SBOX_CHANGE_API_TABLE = 'SBOX_CHANGE_API_TABLE';
export function sboxChangeApiTable(data) {
  return {
    type: SBOX_CHANGE_API_TABLE,
    payload: data
  };
}

export function apiTableReducer(state = initialState, action) {
  switch (action.type) {
  case SBOX_CHANGE_API_TABLE:
    return updateObject(state, { data: action.payload });
  default:
    return state;
  }
}
