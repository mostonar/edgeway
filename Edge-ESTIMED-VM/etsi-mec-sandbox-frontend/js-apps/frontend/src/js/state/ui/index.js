/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { updateObject } from '../../util/object-util';
import {
  PAGE_HOME,
  STATUS_SIGNED_OUT
} from '../../app-constants';

const initialState = {
  page: PAGE_HOME,
  userMenuDisplay: false,
  dashboardMenuDisplay: false,
  helpMenuDisplay: false,
  helpOnSignIn: true,
  sandboxName: '',
  currentDialog: '',
  dialogErrorMsg: '',
  currentAlert: null,
  currentDashboard: '',
  signInStatus: STATUS_SIGNED_OUT,
  signInUsername: '',
  signInUserRole: '',
  networkFiles: [],
  networkFileSelected: '',
  networkInfo: '',
  mecApiSelected: '',
  mecApiCopiedToClipboard: false,
  pauseButton: false,
  nbStationaryUe: 0,
  nbLowVelocityUe: 0,
  nbHighVelocityUe: 0,
  stationaryUeList: [],
  lowVelocityUeList: [],
  highVelocityUeList: [],
  mepList: [],
  edgeApps: [],
  appInstancesSelected: [],
  order: 'asc',
  detailedData: null,
  updateUeInProgressCount: -1,
  updateAutomationInProgressCount: -1,
  updateAppInstancesInProgressCount: -1,
  terminationInProgressCount: -1,
  activationInProgressCount: -1,
  activationInProgressScenarioName: ''
};

// Change the current page
const CHANGE_CURRENT_PAGE = 'CHANGE_CURRENT_PAGE';
export function uiChangeCurrentPage(page) {
  return {
    type: CHANGE_CURRENT_PAGE,
    payload: page
  };
}

// Change the user menu display
const CHANGE_USER_MENU_DISPLAY = 'CHANGE_USER_MENU_DISPLAY';
export function uiChangeUserMenuDisplay(display) {
  return {
    type: CHANGE_USER_MENU_DISPLAY,
    payload: display
  };
}

// Change the help menu display
const CHANGE_DASHBOARD_MENU_DISPLAY = 'CHANGE_DASHBOARD_MENU_DISPLAY';
export function uiChangeDashboardMenuDisplay(display) {
  return {
    type: CHANGE_DASHBOARD_MENU_DISPLAY,
    payload: display
  };
}

// Change the help menu display
const CHANGE_HELP_MENU_DISPLAY = 'CHANGE_HELP_MENU_DISPLAY';
export function uiChangeHelpMenuDisplay(display) {
  return {
    type: CHANGE_HELP_MENU_DISPLAY,
    payload: display
  };
}

// Change to help on sign in
const CHANGE_HELP_ON_SIGN_IN = 'CHANGE_HELP_ON_SIGN_IN';
export function uiChangeHelpOnSignIn(helpOnSignIn) {
  return {
    type: CHANGE_HELP_ON_SIGN_IN,
    payload: helpOnSignIn
  };
}

// Change the sandbox name
const CHANGE_SANDBOX_NAME = 'CHANGE_SANDBOX_NAME';
export function uiChangeSandboxName(name) {
  return {
    type: CHANGE_SANDBOX_NAME,
    payload: name
  };
}

const UI_CHANGE_SIGN_IN_STATUS = 'UI_CHANGE_SIGN_IN_STATUS';
export function uiChangeSignInStatus(status) {
  return {
    type: UI_CHANGE_SIGN_IN_STATUS,
    payload: status
  };
}

const UI_CHANGE_SIGN_IN_USER_NAME = 'UI_CHANGE_SIGN_IN_USER_NAME';
export function uiChangeSignInUsername(name) {
  return {
    type: UI_CHANGE_SIGN_IN_USER_NAME,
    payload: name
  };
}

const UI_CHANGE_SIGN_IN_USER_ROLE = 'UI_CHANGE_SIGN_IN_USER_ROLE';
export function uiChangeSignInUserRole(role) {
  return {
    type: UI_CHANGE_SIGN_IN_USER_ROLE,
    payload: role
  };
}

const UI_CHANGE_SANDBOX = 'UI_CHANGE_SANDBOX';
export function uiChangeSandbox(name) {
  return {
    type: UI_CHANGE_SANDBOX,
    payload: name
  };
}

const UI_CHANGE_CURRENT_DIALOG = 'UI_CHANGE_CURRENT_DIALOG';
export function uiChangeCurrentDialog(type) {
  return {
    type: UI_CHANGE_CURRENT_DIALOG,
    payload: type
  };
}

const UI_CHANGE_DIALOG_ERROR_MSG = 'UI_CHANGE_DIALOG_ERROR_MSG';
export function uiChangeDialogErrorMsg(msg) {
  return {
    type: UI_CHANGE_DIALOG_ERROR_MSG,
    payload: msg
  };
}

const UI_CHANGE_CURRENT_ALERT = 'UI_CHANGE_CURRENT_ALERT';
export function uiChangeCurrentAlert(alert) {
  return {
    type: UI_CHANGE_CURRENT_ALERT,
    payload: alert
  };
}

const UI_CHANGE_CURRENT_DASHBOARD = 'UI_CHANGE_CURRENT_DASHBOARD';
export function uiChangeCurrentDashboard(label) {
  return {
    type: UI_CHANGE_CURRENT_DASHBOARD,
    payload: label
  };
}

const UI_SANDBOX_CHANGE_MAP_CFG = 'UI_SANDBOX_CHANGE_MAP_CFG';
export function uiSandboxChangeMapCfg(cfg) {
  return {
    type: UI_SANDBOX_CHANGE_MAP_CFG,
    payload: cfg
  };
}

const UI_SANDBOX_CHANGE_NETWORK_FILE_SELECTED = 'UI_SANDBOX_CHANGE_NETWORK_FILE_SELECTED';
export function uiSandboxChangeNetworkFileSelected(name) {
  return {
    type: UI_SANDBOX_CHANGE_NETWORK_FILE_SELECTED,
    payload: name
  };
}

const UI_SANDBOX_CHANGE_NETWORK_INFO = 'UI_SANDBOX_CHANGE_NETWORK_INFO';
export function uiSandboxChangeNetworkInfo(value) {
  return {
    type: UI_SANDBOX_CHANGE_NETWORK_INFO,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_MEC_API_SELECTED = 'UI_SANDBOX_CHANGE_MEC_API_SELECTED';
export function uiSandboxChangeMecApiSelected(name) {
  return {
    type: UI_SANDBOX_CHANGE_MEC_API_SELECTED,
    payload: name
  };
}

const UI_SANDBOX_CHANGE_MEC_API_COPIED_TO_CLIPBOARD = 'UI_SANDBOX_CHANGE_MEC_API_COPIED_TO_CLIPBOARD';
export function uiSandboxChangeMecApiCopiedToClipboard(value) {
  return {
    type: UI_SANDBOX_CHANGE_MEC_API_COPIED_TO_CLIPBOARD,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_API_DETAILED_DATA = 'UI_SANDBOX_CHANGE_API_DETAILED_DATA';
export function uiSandboxChangeApiDetailedData(data) {
  return {
    type: UI_SANDBOX_CHANGE_API_DETAILED_DATA,
    payload: data
  };
}

const UI_SANDBOX_CHANGE_NETWORK_FILES_LIST = 'UI_SANDBOX_CHANGE_NETWORK_FILES_LIST';
export function uiSandboxChangeNetworkFilesList(networkFiles) {
  return {
    type: UI_SANDBOX_CHANGE_NETWORK_FILES_LIST,
    payload: networkFiles
  };
}

const UI_SANDBOX_CHANGE_PAUSE_BUTTON = 'UI_SANDBOX_CHANGE_PAUSE_BUTTON';
export function uiSandboxChangePauseButton(checked) {
  return {
    type: UI_SANDBOX_CHANGE_PAUSE_BUTTON,
    payload: checked
  };
}

const UI_SANDBOX_CHANGE_NB_STATIONARY_UE = 'UI_SANDBOX_CHANGE_NB_STATIONARY_UE';
export function uiSandboxChangeNbStationaryUe(value) {
  return {
    type: UI_SANDBOX_CHANGE_NB_STATIONARY_UE,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_NB_LOW_VELOCITY_UE = 'UI_SANDBOX_CHANGE_NB_LOW_VELOCITY_UE';
export function uiSandboxChangeNbLowVelocityUe(value) {
  return {
    type: UI_SANDBOX_CHANGE_NB_LOW_VELOCITY_UE,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_NB_HIGH_VELOCITY_UE = 'UI_SANDBOX_CHANGE_NB_HIGH_VELOCITY_UE';
export function uiSandboxChangeNbHighVelocityUe(value) {
  return {
    type: UI_SANDBOX_CHANGE_NB_HIGH_VELOCITY_UE,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_STATIONARY_UE_LIST = 'UI_SANDBOX_CHANGE_STATIONARY_UE_LIST';
export function uiSandboxChangeStationaryUeList(value) {
  return {
    type: UI_SANDBOX_CHANGE_STATIONARY_UE_LIST,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_LOW_VELOCITY_UE_LIST = 'UI_SANDBOX_CHANGE_LOW_VELOCITY_UE_LIST';
export function uiSandboxChangeLowVelocityUeList(value) {
  return {
    type: UI_SANDBOX_CHANGE_LOW_VELOCITY_UE_LIST,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_HIGH_VELOCITY_UE_LIST = 'UI_SANDBOX_CHANGE_HIGH_VELOCITY_UE_LIST';
export function uiSandboxChangeHighVelocityUeList(value) {
  return {
    type: UI_SANDBOX_CHANGE_HIGH_VELOCITY_UE_LIST,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_MEP_LIST = 'UI_SANDBOX_CHANGE_MEP_LIST';
export function uiSandboxChangeMepList(value) {
  return {
    type: UI_SANDBOX_CHANGE_MEP_LIST,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_EDGE_APP_LIST = 'UI_SANDBOX_CHANGE_EDGE_APP_LIST';
export function uiSandboxChangeEdgeAppList(value) {
  return {
    type: UI_SANDBOX_CHANGE_EDGE_APP_LIST,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_APP_INSTANCES_SELECTED = 'UI_SANDBOX_CHANGE_APP_INSTANCES_SELECTED';
export function uiSandboxChangeAppInstancesSelected(value) {
  return {
    type: UI_SANDBOX_CHANGE_APP_INSTANCES_SELECTED,
    payload: value
  };
}

const UI_SANDBOX_CHANGE_UPDATE_UE_IN_PROGRESS_COUNT = 'UI_SANDBOX_CHANGE_UPDATE_UE_IN_PROGRESS_COUNT';
export function uiSandboxChangeUpdateUeInProgressCount(count) {
  return {
    type: UI_SANDBOX_CHANGE_UPDATE_UE_IN_PROGRESS_COUNT,
    payload: count
  };
}

const UI_SANDBOX_CHANGE_UPDATE_AUTOMATION_IN_PROGRESS_COUNT = 'UI_SANDBOX_CHANGE_UPDATE_AUTOMATION_IN_PROGRESS_COUNT';
export function uiSandboxChangeUpdateAutomationInProgressCount(count) {
  return {
    type: UI_SANDBOX_CHANGE_UPDATE_AUTOMATION_IN_PROGRESS_COUNT,
    payload: count
  };
}

const UI_SANDBOX_CHANGE_UPDATE_APP_INSTANCES_IN_PROGRESS_COUNT = 'UI_SANDBOX_CHANGE_UPDATE_APP_INSTANCES_IN_PROGRESS_COUNT';
export function uiSandboxChangeUpdateAppInstancesInProgressCount(count) {
  return {
    type: UI_SANDBOX_CHANGE_UPDATE_APP_INSTANCES_IN_PROGRESS_COUNT,
    payload: count
  };
}

const UI_SANDBOX_CHANGE_TERMINATION_IN_PROGRESS_COUNT = 'UI_SANDBOX_CHANGE_TERMINATION_IN_PROGRESS_COUNT';
export function uiSandboxChangeTerminationInProgressCount(count) {
  return {
    type: UI_SANDBOX_CHANGE_TERMINATION_IN_PROGRESS_COUNT,
    payload: count
  };
}

const UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_COUNT = 'UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_COUNT';
export function uiSandboxChangeActivationInProgressCount(count) {
  return {
    type: UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_COUNT,
    payload: count
  };
}

const UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_SCENARIO_NAME = 'UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_SCENARIO_NAME';
export function uiSandboxChangeActivationInProgressScenarioName(name) {
  return {
    type: UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_SCENARIO_NAME,
    payload: name
  };
}

export default function uiReducer(state = initialState, action) {
  switch (action.type) {
  case CHANGE_CURRENT_PAGE:
    return updateObject(state, { page: action.payload });
  case CHANGE_USER_MENU_DISPLAY:
    return updateObject(state, { userMenuDisplay: action.payload });
  case CHANGE_DASHBOARD_MENU_DISPLAY:
    return updateObject(state, { dashboardMenuDisplay: action.payload });
  case CHANGE_HELP_MENU_DISPLAY:
    return updateObject(state, { helpMenuDisplay: action.payload });
  case CHANGE_HELP_ON_SIGN_IN:
    return updateObject(state, { helpOnSignIn: action.payload });
  case CHANGE_SANDBOX_NAME:
    return updateObject(state, { sandboxName: action.payload });
  case UI_CHANGE_SIGN_IN_STATUS:
    return updateObject(state, { signInStatus: action.payload });
  case UI_CHANGE_SIGN_IN_USER_NAME:
    return updateObject(state, { signInUsername: action.payload });
  case UI_CHANGE_SIGN_IN_USER_ROLE:
    return updateObject(state, { signInUserRole: action.payload });
  case UI_CHANGE_SANDBOX:
    return updateObject(state, { sandbox: action.payload });
  case UI_CHANGE_CURRENT_DIALOG:
    return updateObject(state, { currentDialog: action.payload });
  case UI_CHANGE_DIALOG_ERROR_MSG:
    return updateObject(state, { dialogErrorMsg: action.payload });
  case UI_CHANGE_CURRENT_ALERT:
    return updateObject(state, { currentAlert: action.payload });
  case UI_CHANGE_CURRENT_DASHBOARD:
    return updateObject(state, { currentDashboard: action.payload });
  case UI_SANDBOX_CHANGE_MAP_CFG:
    return updateObject(state, { mapCfg: action.payload });
  case UI_SANDBOX_CHANGE_NETWORK_FILES_LIST:
    return updateObject(state, { networkFiles: action.payload });
  case UI_SANDBOX_CHANGE_UPDATE_UE_IN_PROGRESS_COUNT:
    return updateObject(state, { updateUeInProgressCount: action.payload });
  case UI_SANDBOX_CHANGE_UPDATE_AUTOMATION_IN_PROGRESS_COUNT:
    return updateObject(state, { updateAutomationInProgressCount: action.payload });
  case UI_SANDBOX_CHANGE_UPDATE_APP_INSTANCES_IN_PROGRESS_COUNT:
    return updateObject(state, { updateAppInstancesInProgressCount: action.payload });
  case UI_SANDBOX_CHANGE_TERMINATION_IN_PROGRESS_COUNT:
    return updateObject(state, { terminationInProgressCount: action.payload });
  case UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_COUNT:
    return updateObject(state, { activationInProgressCount: action.payload });
  case UI_SANDBOX_CHANGE_ACTIVATION_IN_PROGRESS_SCENARIO_NAME:
    return updateObject(state, { activationInProgressScenarioName: action.payload });
  case UI_SANDBOX_CHANGE_NETWORK_FILE_SELECTED:
    return updateObject(state, { networkFileSelected: action.payload });
  case UI_SANDBOX_CHANGE_NETWORK_INFO:
    return updateObject(state, { networkInfo: action.payload });
  case UI_SANDBOX_CHANGE_MEC_API_SELECTED:
    return updateObject(state, { mecApiSelected: action.payload });
  case UI_SANDBOX_CHANGE_MEC_API_COPIED_TO_CLIPBOARD:
    return updateObject(state, { mecApiCopiedToClipboard: action.payload });
  case UI_SANDBOX_CHANGE_API_DETAILED_DATA:
    return updateObject(state, { detailedData: action.payload });
  case UI_SANDBOX_CHANGE_PAUSE_BUTTON:
    return updateObject(state, { pauseButton: action.payload });
  case UI_SANDBOX_CHANGE_NB_STATIONARY_UE:
    return updateObject(state, { nbStationaryUe: action.payload });
  case UI_SANDBOX_CHANGE_NB_LOW_VELOCITY_UE:
    return updateObject(state, { nbLowVelocityUe: action.payload });
  case UI_SANDBOX_CHANGE_NB_HIGH_VELOCITY_UE:
    return updateObject(state, { nbHighVelocityUe: action.payload });
  case UI_SANDBOX_CHANGE_STATIONARY_UE_LIST:
    return updateObject(state, { stationaryUeList: action.payload });
  case UI_SANDBOX_CHANGE_LOW_VELOCITY_UE_LIST:
    return updateObject(state, { lowVelocityUeList: action.payload });
  case UI_SANDBOX_CHANGE_HIGH_VELOCITY_UE_LIST:
    return updateObject(state, { highVelocityUeList: action.payload });
  case UI_SANDBOX_CHANGE_MEP_LIST:
    return updateObject(state, { mepList: action.payload });
  case UI_SANDBOX_CHANGE_EDGE_APP_LIST:
    return updateObject(state, { edgeApps: action.payload });
  case UI_SANDBOX_CHANGE_APP_INSTANCES_SELECTED:
    return updateObject(state, { appInstancesSelected: action.payload });

  default:
    return state;
  }
}
