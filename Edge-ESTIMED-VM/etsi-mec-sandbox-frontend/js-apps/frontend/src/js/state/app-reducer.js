/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { combineReducers } from 'redux';

import uiReducer from './ui';
import sboxReducer from './sbox';

const appStateReducer = combineReducers({
  ui: uiReducer,
  sbox: sboxReducer
});

const UI_SET_DEFAULT_STATE = 'UI_SET_DEFAULT_STATE';
export function appSetDefaultState() {
  return {
    type: UI_SET_DEFAULT_STATE,
    payload: ''
  };
}

const appReducer = (state, action) => {
  switch (action.type) {
  case UI_SET_DEFAULT_STATE:
    return appStateReducer(undefined, action);
  default:
    return appStateReducer(state, action);
  }
};

export default appReducer;
