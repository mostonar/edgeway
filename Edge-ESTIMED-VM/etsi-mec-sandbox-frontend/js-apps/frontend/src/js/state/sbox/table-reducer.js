/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';

const initialState = {
  data: [],
  entries: {},
  refresh: () => {}
};

const SBOX_CHANGE_TABLE = 'SBOX_CHANGE_TABLE';
export function sboxChangeTable(table) {
  return {
    type: SBOX_CHANGE_TABLE,
    payload: table
  };
}

export function tableReducer(state = initialState, action) {
  switch (action.type) {
  case SBOX_CHANGE_TABLE:
    return updateObject({}, action.payload);
  default:
    return state;
  }
}
