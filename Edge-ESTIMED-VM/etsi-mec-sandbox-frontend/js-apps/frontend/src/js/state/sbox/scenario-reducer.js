/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';
import { NO_SCENARIO_NAME } from '../../app-constants';

const initialState = {
  name: NO_SCENARIO_NAME
};

// SBOX_CHANGE_SCENARIO
const SBOX_CHANGE_SCENARIO = 'SBOX_CHANGE_SCENARIO';
export function sboxChangeScenario(scenario) {
  return {
    type: SBOX_CHANGE_SCENARIO,
    payload: scenario
  };
}

// SBOX_CHANGE_SCENARIO_NAME
const SBOX_CHANGE_SCENARIO_NAME = 'CFG_CHANGE_SCENARIO_NAME';
export function sboxChangeScenarioName(name) {
  return {
    type: SBOX_CHANGE_SCENARIO_NAME,
    payload: name
  };
}

export function scenarioReducer(state = initialState, action) {
  switch (action.type) {
  case SBOX_CHANGE_SCENARIO_NAME:
    return updateObject(state, { name: action.payload });
  case SBOX_CHANGE_SCENARIO:
    return action.payload;
  default:
    return state;
  }
}
