/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { combineReducers } from 'redux';

import { scenarioReducer } from './scenario-reducer';
import { mapReducer } from './map-reducer';
import { tableReducer } from './table-reducer';
import { apiTableReducer } from './api-table-reducer';
import { appInstanceTableReducer } from './app-instance-table-reducer';
import { svcTableReducer } from './svc-table-reducer';

export * from './scenario-reducer';
export * from './map-reducer';
export * from './table-reducer';
export * from './api-table-reducer';
export * from './app-instance-table-reducer';
export * from './svc-table-reducer';

const sboxReducer = combineReducers({
  scenario: scenarioReducer,
  map: mapReducer,
  table: tableReducer,
  apiTable: apiTableReducer,
  appInstanceTable: appInstanceTableReducer,
  svcTable: svcTableReducer
});

export default sboxReducer;
