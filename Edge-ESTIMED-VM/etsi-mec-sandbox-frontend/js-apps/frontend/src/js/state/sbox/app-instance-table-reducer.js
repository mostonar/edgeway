/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';

const initialState = {
  data: []
};

const SBOX_CHANGE_APP_INSTANCE_TABLE = 'SBOX_CHANGE_APP_INSTANCE_TABLE';
export function sboxChangeAppInstanceTable(data) {
  return {
    type: SBOX_CHANGE_APP_INSTANCE_TABLE,
    payload: data
  };
}

export function appInstanceTableReducer(state = initialState, action) {
  switch (action.type) {
  case SBOX_CHANGE_APP_INSTANCE_TABLE:
    return updateObject(state, { data: action.payload });
  default:
    return state;
  }
}
