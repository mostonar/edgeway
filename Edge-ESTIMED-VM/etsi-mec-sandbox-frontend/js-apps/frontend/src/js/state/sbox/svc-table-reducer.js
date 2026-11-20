/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';

const initialState = {
  data: []
};

const SBOX_CHANGE_SVC_TABLE = 'SBOX_CHANGE_SVC_TABLE';
export function sboxChangeSvcTable(data) {
  return {
    type: SBOX_CHANGE_SVC_TABLE,
    payload: data
  };
}

export function svcTableReducer(state = initialState, action) {
  switch (action.type) {
  case SBOX_CHANGE_SVC_TABLE:
    return updateObject(state, { data: action.payload });
  default:
    return state;
  }
}
