/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

// Will persist the app state between browser refresh
// import * as stringify from 'json-stringify-safe';
// import {version} from './package.json';

// /*global __VERSION__*/
// const STATE_KEY = 'ETSI-MEC-Sandbox-frontend' + (__VERSION__ ? ':' + __VERSION__ : '') + ':state';
const STATE_KEY = 'ETSI-MEC-Sandbox-frontend:state';

export function saveState(state) {
  try {
    let serializedState = JSON.stringify(state);
    localStorage.setItem(STATE_KEY, serializedState);
  } catch (e) {
    // TODO: consider showing an alert.
    console.log('Error while saving app state: ', e);
  }
}

export function loadState() {
  try {
    let serializedState = localStorage.getItem(STATE_KEY);

    if (serializedState === null) {
      return this.initializeState();
    }

    return JSON.parse(serializedState);
  } catch (err) {
    return null;
  }
}
