/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

// Import CSS
import 'material-design-icons/iconfont/material-icons.css';
import 'leaflet/dist/leaflet.css';
import 'mapbox-gl/dist/mapbox-gl.css';
import 'ionicons/scss/ionicons.scss';
import '../css/app-controller.scss';

// Import module dependencies
import 'material-design-icons';
import React from 'react';
import ReactDOM from 'react-dom';
import thunk from 'redux-thunk';
import { createStore, applyMiddleware } from 'redux';
import { Provider } from 'react-redux';
import appReducer from './state/app-reducer';

import { saveState, loadState } from './util/persist';


// UI Components
import AppContainer from './containers/app-container';

import log from './util/logger';

// Initialize variables and listeners when document ready

// Get state from local storage
// Set state to 'undefined' to use default values
const SBOX_LOCAL_STORAGE_STATE = 'mec-sbox-state';

// Remote logger for debugging
const originalLog = console.log;
const originalError = console.error;
const originalWarn = console.warn;
const sendLog = (type, args) => {
  try {
    const msg = encodeURIComponent(type + ': ' + Array.from(args).map(a => typeof a === 'object' ? JSON.stringify(a) : String(a)).join(' '));
    fetch('/platform-ctrl/v1/frontend-log?msg=' + msg).catch(() => {});
  } catch (e) {}
};
console.log = function() { originalLog.apply(console, arguments); sendLog('LOG', arguments); };
console.error = function() { originalError.apply(console, arguments); sendLog('ERR', arguments); };
console.warn = function() { originalWarn.apply(console, arguments); sendLog('WARN', arguments); };

import { getWebInstrumentations, initializeFaro } from '@grafana/faro-web-sdk';
import { TracingInstrumentation } from '@grafana/faro-web-tracing';
import { telemetryMiddleware } from './state/telemetry-middleware';

// Initialize Grafana Faro
initializeFaro({
  url: window.location.origin + '/telemetry/collect', // Uses the Ingress route
  app: {
    name: 'mec-sandbox-frontend',
    version: '1.0.0',
    environment: 'development'
  },
  instrumentations: [
    ...getWebInstrumentations(),
    new TracingInstrumentation()
  ]
});

// Setup logging

var loadedState = loadState();

// Redux logger middleware

const logger = store => next => action => {
  log.debug(`logger - action: ${action.type}. payload: `, action.payload);
  // log.trace('state: ', store.getState()); // Uncomment for detailed state logging
  return next(action);
};

// Create state store
const appStore = createStore(
  appReducer,
  loadedState ? loadedState : undefined,
  applyMiddleware(
    thunk,
    logger,
    telemetryMiddleware
  )
);
window.appStore = appStore;

appStore.subscribe(() => {
  var curState = appStore.getState();

  // Filter state to be persisted
  // NOTE: do not modify current state!
  var filteredState = {
    monitor: curState.monitor,
    settings: curState.settings,
    ui: curState.ui
  };

  saveState(filteredState);
});

// Monitor Page
let appContainerPlaceholder = document.getElementById('app-container');
ReactDOM.render(
  <Provider store={appStore}>
    <AppContainer />
  </Provider>,
  appContainerPlaceholder
);
