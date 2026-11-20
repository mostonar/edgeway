/*
 * Copyright (c) 2022-2025 ETSI.  All rights reserved.
 */

import _ from 'lodash';
import { connect } from 'react-redux';
import log from '../util/logger';
import React, { Component } from 'react';
import { StatusCodes } from 'http-status-codes';
import { TopAppBarFixedAdjust } from '@rmwc/top-app-bar';

import TopBarContainer from './top-bar-container';
import HomePageContainer from './home/home-page-container';
import SandboxPageContainer from './sandbox/sandbox-page-container';
import DashboardPageContainer from './dashboard/dashboard-page-container';
import Footer from '../components/footer';
import { updateObject } from '../util/object-util';

// import SignInDialog from '../components/dialogs/sign-in-dialog';
import SignInOAuthDialog from '../components/dialogs/sign-in-oauth-dialog';
import SignInWaitDialog from '../components/dialogs/sign-in-wait-dialog';
import SessionTerminatedDialog from '../components/dialogs/session-terminated-dialog';
import HelpGettingStartedDialog from '../components/dialogs/help-getting-started-dialog';
import ConfirmDeleteAppDialog from '../components/dialogs/confirm-delete-app-dialog';
import CreateNewAppDialog from '../components/dialogs/create-new-app-dialog';
import VersionDialog from '../components/dialogs/version-dialog';
import SslTrustErrorDialog from '../components/dialogs/ssl-trust-error-dialog';

import * as meepAuthSvcRestApiClient from '../../../../../js-packages/meep-auth-svc-client/src/index.js';
import * as meepPlatformCtrlRestApiClient from '../../../../../js-packages/meep-platform-ctrl-client/src/index.js';
import * as meepSandboxCtrlRestApiClient from '../../../../../js-packages/meep-sandbox-ctrl-client/src/index.js';
import * as meepMetricsEngineRestApiClient from '../../../../../js-packages/meep-metrics-engine-client/src/index.js';
import * as meepGisEngineRestApiClient from '../../../../../js-packages/meep-gis-engine-client/src/index.js';
import * as meepMonEngineRestApiClient from '../../../../../js-packages/meep-mon-engine-client/src/index.js';

import {
  HOST_PATH,
  PAGE_HOME,
  PAGE_SANDBOX,
  PAGE_DASHBOARD,
  DEFAULT_NO_NETWORK_FILE_SELECTED,
  MIN_SCREEN_WIDTH,
  MIN_SCREEN_HEIGHT,
  DIALOG_SIGN_IN,
  DIALOG_SIGN_IN_WAIT,
  DIALOG_SESSION_TERMINATED,
  DIALOG_HELP_GETTING_STARTED,
  DIALOG_CONFIRM_DELETE_APP,
  DIALOG_CREATE_NEW_APP,
  DIALOG_VERSION,
  DIALOG_SSL_TRUST_ERROR,
  ALERT_DEGRADED_NETWORK,
  STATUS_SIGNED_IN,
  STATUS_SIGNING_IN,
  STATUS_SIGNED_OUT,
  EDGE_APP_ENABLE_COUNT_MAX,
  BLACKOUT_TIME,
  EDGE_APP_DISABLE_COUNT_MAX,
  NO_SCENARIO_NAME
} from '../app-constants';

import {
  parseScenario
} from '../util/scenario-utils';

import {
  uiChangeCurrentDialog,
  uiChangeDialogErrorMsg,
  uiChangeHelpOnSignIn,
  uiChangeCurrentAlert,
  uiChangeCurrentPage,
  uiChangeSandboxName,
  uiSandboxChangeNetworkFileSelected,
  uiSandboxChangeNetworkInfo,
  uiSandboxChangeNbStationaryUe,
  uiSandboxChangeNbLowVelocityUe,
  uiSandboxChangeNbHighVelocityUe,
  uiSandboxChangeStationaryUeList,
  uiSandboxChangeLowVelocityUeList,
  uiSandboxChangeHighVelocityUeList,
  uiSandboxChangeMepList,
  uiSandboxChangeEdgeAppList,
  uiSandboxChangeAppInstancesSelected,
  uiSandboxChangePauseButton,
  uiSandboxChangeApiDetailedData,
  uiChangeSignInStatus,
  uiChangeSignInUsername,
  uiChangeSignInUserRole,
  uiSandboxChangeUpdateUeInProgressCount,
  uiSandboxChangeUpdateAutomationInProgressCount,
  uiSandboxChangeUpdateAppInstancesInProgressCount,
  uiSandboxChangeTerminationInProgressCount,
  uiSandboxChangeActivationInProgressCount,
  uiSandboxChangeActivationInProgressScenarioName
} from '../state/ui';

import {
  sboxChangeScenario,
  sboxChangeTable,
  sboxChangeMap,
  sboxChangeMapUeList,
  sboxChangeMapPoaList,
  sboxChangeMapComputeList,
  sboxChangeApiTable,
  sboxChangeAppInstanceTable,
  sboxChangeSvcTable
} from '../state/sbox';

const SBOX_ERR_COUNT_MAX = 5;
const SBOX_CREATE_TIMEOUT = 30000; // 30 sec
const SBOX_REFRESH_INTERVAL = 1000; // 1 sec
const SESSION_KEEPALIVE_INTERVAL = 600000; // 10 min

// REST API Clients
var basepathAuthSvc = '';
var basepathPlatformCtrl = '';
var basepathSandboxCtrl = '';
var basepathMetricsEngineCtrl = '';
var basepathGisEngineCtrl = '';
var basepathMonEngine = '';

const apiTableMaxSize = 100;
var metricsQuery = {
  /*  tags: [{
        name: 'logger_name',
        value: 'meep-loc-serv,meep-federation,meep-rnis,meep-dai,meep-vis,meep-iot,meep-sss,meep-federation,meep-wais,meep-app-enablement'
      }],
    */
  fields: ['id', 'endpoint', 'url', 'method', 'resp_code', 'resp_body', 'body', 'proc_time', 'logger_name', 'direction'],
  scope: {
    limit: apiTableMaxSize,
    duration: '2s'
  }
};

function importAll(r) {
  return r.keys().map(r);
}


//forcing imports of every file in img directory
const images = importAll(require.context('../../img', false, /\.(png|jpe?g|svg)$/));


class AppContainer extends Component {
  constructor(props) {
    super(props);

    this.creationSandboxTimer = null;
    this.sboxPageRefreshIntervalTimer = null;
    this.sandboxErrorCount = 0;
    this.sessionKeepaliveTimer = null;
    this.refreshApiConsole = false;
    this.refreshUePaths = false;
    this.uePathUpdateRequired = false;

    basepathAuthSvc = HOST_PATH + '/auth/v1';
    meepAuthSvcRestApiClient.ApiClient.instance.basePath = basepathAuthSvc.replace(/\/+$/, '');
    basepathPlatformCtrl = HOST_PATH + '/platform-ctrl/v1';
    meepPlatformCtrlRestApiClient.ApiClient.instance.basePath = basepathPlatformCtrl.replace(/\/+$/, '');
    basepathMonEngine = HOST_PATH + '/mon-engine/v1';
    meepMonEngineRestApiClient.ApiClient.instance.basePath = basepathMonEngine.replace(/\/+$/, '');
    this.meepAuthApi = new meepAuthSvcRestApiClient.AuthApi();
    this.meepScenarioConfigurationApi = new meepPlatformCtrlRestApiClient.ScenarioConfigurationApi();
    this.meepSandboxControlApi = new meepPlatformCtrlRestApiClient.SandboxControlApi();
    this.meepActiveScenarioApi = new meepSandboxCtrlRestApiClient.ActiveScenarioApi();
    this.meepEventsApi = new meepSandboxCtrlRestApiClient.EventsApi();
    this.meepApplicationsApi = new meepSandboxCtrlRestApiClient.ApplicationsApi();
    this.meepServicesApi = new meepSandboxCtrlRestApiClient.ServicesApi();
    this.meepMetricsEngineApi = new meepMetricsEngineRestApiClient.MetricsApi();
    this.meepGisAutomationApi = new meepGisEngineRestApiClient.AutomationApi();
    this.meepGeoDataApi = new meepGisEngineRestApiClient.GeospatialDataApi();
    this.meepMonEngineApi = new meepMonEngineRestApiClient.PodStatesApi();

    // Intercept 401 Unauthorized responses to gracefully handle session expiry
    const addAuthInterceptor = (client) => {
      const instance = client.ApiClient.instance;
      const originalCallApi = instance.callApi.bind(instance);
      instance.callApi = (path, httpMethod, pathParams, queryParams, collectionQueryParams, headerParams, formParams, bodyParam, authNames, contentTypes, accepts, returnType, callback) => {
        const wrappedCallback = (error, data, response) => {
          if (error && error.status === StatusCodes.UNAUTHORIZED) {
            if (this.props.signInStatus === STATUS_SIGNED_IN) {
              this.logout();
              this.props.changeCurrentDialog(DIALOG_SESSION_TERMINATED);
            }
          }
          if (callback) {
            callback(error, data, response);
          }
        };
        return originalCallApi(path, httpMethod, pathParams, queryParams, collectionQueryParams, headerParams, formParams, bodyParam, authNames, contentTypes, accepts, returnType, wrappedCallback);
      };
    };

    addAuthInterceptor(meepAuthSvcRestApiClient);
    addAuthInterceptor(meepPlatformCtrlRestApiClient);
    addAuthInterceptor(meepSandboxCtrlRestApiClient);
    addAuthInterceptor(meepMetricsEngineRestApiClient);
    addAuthInterceptor(meepGisEngineRestApiClient);
    addAuthInterceptor(meepMonEngineRestApiClient);

    // Initialize undefined states
    if (this.props.terminationInProgressCount === undefined) {
      this.props.changeTerminationInProgressCount(-1);
    }
    if (this.props.activationInProgressCount === undefined) {
      this.props.changeActivationInProgressCount(-1);
    }
    if (this.props.activationInProgressScenarioName === undefined) {
      this.props.changeActivationInProgressScenarioName('');
    }
    if (this.props.updateUeInProgressCount === undefined) {
      this.props.changeUpdateUeInProgressCount(-1);
    }
    if (this.props.updateAutomationInProgressCount === undefined) {
      this.props.changeUpdateAutomationInProgressCount(-1);
    }
    if (this.props.updateAppInstancesInProgressCount === undefined) {
      this.props.changeUpdateAppInstancesInProgressCount(-1);
    }
    if (this.props.stationaryUeList === undefined) {
      this.props.changeStationaryUeList([]);
    }
    if (this.props.lowVelocityUeList === undefined) {
      this.props.changeLowVelocityUeList([]);
    }
    if (this.props.highVelocityUeList === undefined) {
      this.props.changeHighVelocityUeList([]);
    }
    if (this.props.mepList === undefined) {
      this.props.changeMepList([]);
    }
    if (this.props.appInstancesSelected === undefined) {
      this.props.changeAppInstancesSelected([]);
    }
    if (this.props.helpOnSignIn === undefined) {
      this.props.changeHelpOnSignIn(true);
    }

    // Handle OAuth login in progress
    if (this.props.signInStatus === STATUS_SIGNING_IN) {
      // Get Sandbox name from query params
      let params = (new URL(document.location)).searchParams;
      let sandboxName = params.get('sbox');
      if (sandboxName) {
        // Store Sandbox name
        this.props.changeSandboxName(sandboxName);

        // Get user name & role to configure frontend
        let userName = params.get('user');
        if (userName) {
          this.props.changeSignInUsername(userName);
        }
        let userRole = params.get('role');
        if (userRole) {
          this.props.changeSignInUserRole(userRole);
        }

        // Reset network
        this.props.changeNetworkFileSelected(DEFAULT_NO_NETWORK_FILE_SELECTED);

        // Set current dialog to signing in
        this.props.changeCurrentDialog(DIALOG_SIGN_IN_WAIT);

        // Remove Query params from URL now that we have stored them
        window.history.replaceState({}, document.title, '/');
      } else {
        // Sign in failed
        this.logout();
        this.props.changeCurrentDialog(null);
        this.props.changeSignInStatus(STATUS_SIGNED_OUT);
      }
    }
  }

  componentDidMount() {
    log.info('componentDidMount: AppContainer mounted');
    document.title = 'EdgeNative - Connector';
    this.setBasePaths();
    this.startTimers();
    this.monitorTabFocus();
  }

  componentDidUpdate(prevProps) {
    // If number of moving UEs has increased, request a UE path refresh asap
    if (this.props.nbLowVelocityUe > prevProps.nbLowVelocityUe ||
            this.props.nbHighVelocityUe > prevProps.nbHighVelocityUe) {
      this.uePathUpdateRequired = true;
    }

    // Set new basepaths if sandbox name updated
    if (this.props.sandboxName !== prevProps.sandboxName) {
      this.setBasePaths();
    }

    // Trigger API console refresh when switching back to sandbox page
    if (this.props.page === PAGE_SANDBOX && prevProps.page !== PAGE_SANDBOX) {
      this.refreshApiConsole = true;
    }

    // Clear scenario immediately when termination starts to prevent UI from temporarily restoring the old scenario
    if (this.props.terminationInProgressCount > 0 && prevProps.terminationInProgressCount === -1) {
      this.updateScenario(null);
      this.props.changeApiTable(null);
      this.props.changeApiDetailedData(null);
      this.props.changeAppInstanceTable([]);
      this.props.changeSvcTable([]);
    }
  }

  // Timers
  startTimers() {
    this.refreshApiConsole = true;
    this.startSandboxPageRefresh();
  }
  stopTimers() {
    this.stopSandboxPageRefresh();
  }

  startSandboxCreationTimer() {
    // Delay to let sandbox creation complete
    this.creationSandboxTimer = setTimeout(() => {
      this.createSandboxTimerExpiry();
    }, SBOX_CREATE_TIMEOUT);
  }
  stopSandboxCreationTimer() {
    if (this.creationSandboxTimer) {
      clearTimeout(this.creationSandboxTimer);
      this.creationSandboxTimer = null;
    }
  }

  // Sandbox page refresh
  startSandboxPageRefresh() {
    if (!this.sboxPageRefreshIntervalTimer) {
      this.sboxPageRefreshIntervalTimer = setInterval(
        () => {
          // Sign In state
          if (this.props.signInStatus === STATUS_SIGNED_IN) {
            this.checkSignedIn();
          } else if (this.props.signInStatus === STATUS_SIGNING_IN) {
            this.checkSandboxState();
          }

          // Refresh sandbox page
          if (this.props.page === PAGE_SANDBOX) {
            if (this.props.sandboxName) {
              this.checkScenarioState();
              this.refreshScenario();
              this.refreshMap();
              // Only update while scenario is running
              if (this.props.scenario && (this.props.scenario.name !== NO_SCENARIO_NAME)) {
                this.refreshAutomation();
                this.refreshMetricsTable();
                this.refreshAppInstancesTable();
                this.refreshServices();
              }
            }
          }

          // Decrement progress counters
          this.decrementProgressCounters();
        },
        SBOX_REFRESH_INTERVAL
      );
    }
  }
  stopSandboxPageRefresh() {
    if (this.sboxPageRefreshIntervalTimer) {
      clearInterval(this.sboxPageRefreshIntervalTimer);
      this.sboxPageRefreshIntervalTimer = null;
    }
    this.stopSandboxCreationTimer();
  }

  // Session Keep-alive
  startSessionKeepaliveTimer() {
    if (!this.sessionKeepaliveTimer) {
      // Send heartbeat
      this.sendKeepalive();

      // Start keepalive timer
      this.sessionKeepaliveTimer = setInterval(
        () => {
          // Send heartbeat
          this.sendKeepalive();
        },
        SESSION_KEEPALIVE_INTERVAL
      );
    }
  }
  stopSessionKeepaliveTimer() {
    if (this.sessionKeepaliveTimer) {
      clearInterval(this.sessionKeepaliveTimer);
      this.sessionKeepaliveTimer = null;
    }
  }

  monitorTabFocus() {
    var hidden, visibilityChange;
    if (typeof document.hidden !== 'undefined') {
      // Opera 12.10 and Firefox 18 and later support
      hidden = 'hidden';
      visibilityChange = 'visibilitychange';
    } else if (typeof document.msHidden !== 'undefined') {
      hidden = 'msHidden';
      visibilityChange = 'msvisibilitychange';
    } else if (typeof document.webkitHidden !== 'undefined') {
      hidden = 'webkitHidden';
      visibilityChange = 'webkitvisibilitychange';
    }

    const handleVisibilityChange = () => {
      if (document[hidden]) {
        this.stopTimers();
      } else {
        this.startTimers();
      }
    };

    // Warn if the browser doesn't support addEventListener or the Page Visibility API
    if (
      typeof document.addEventListener === 'undefined' ||
            hidden === undefined
    ) {
      // TODO: consider showing an alert
      // console.log('This demo requires a browser, such as Google Chrome or Firefox, that supports the Page Visibility API.');
    } else {
      // Handle page visibility change
      document.addEventListener(
        visibilityChange,
        handleVisibilityChange,
        false
      );
    }
  }

  setBasePaths() {
    basepathSandboxCtrl = HOST_PATH + '/' + this.props.sandboxName + '/sandbox-ctrl/v1';
    meepSandboxCtrlRestApiClient.ApiClient.instance.basePath = basepathSandboxCtrl.replace(/\/+$/, '');
    basepathMetricsEngineCtrl = HOST_PATH + '/' + this.props.sandboxName + '/metrics/v2';
    meepMetricsEngineRestApiClient.ApiClient.instance.basePath = basepathMetricsEngineCtrl.replace(/\/+$/, '');
    basepathGisEngineCtrl = HOST_PATH + '/' + this.props.sandboxName + '/gis/v1';
    meepGisEngineRestApiClient.ApiClient.instance.basePath = basepathGisEngineCtrl.replace(/\/+$/, '');
  }

  setMainContent(targetId) {
    this.props.changeCurrentPage(targetId);
  }

  createSandboxTimerExpiry() {
    if (this.props.signInStatus === STATUS_SIGNING_IN) {
      this.logout();
      this.props.changeCurrentDialog(null);
      this.props.changeSignInStatus(STATUS_SIGNED_OUT);
    }
  }

  closeDialog() {
    this.props.changeCurrentDialog(null);
  }

  signInProcedure() {
    if (this.props.signInStatus === STATUS_SIGNED_IN) {
      this.logout();
    } else {
      this.props.changeDialogErrorMsg('');
      this.props.changeCurrentDialog(DIALOG_SIGN_IN);
    }
  }

  /**
     * Callback function to receive the result of the loginUser operation.
     * @callback module:api/AuthApi~loginUserCallback
     * @param {String} error Error message, if any.
     * @param {module:model/Sandbox} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  loginCb(error, data) {
    if (error) {
      var errMsg;
      switch (error.status) {
      case StatusCodes.UNAUTHORIZED:
        errMsg = 'Invalid credentials. Please try again...';
        break;
      case StatusCodes.SERVICE_UNAVAILABLE:
        errMsg = 'All EdgeNative - Connector sandboxes are currently in use. Please try again later...';
        break;
      default:
        errMsg = 'Login failure. Please try again...';
        break;
      }
      this.props.changeDialogErrorMsg(errMsg);
      this.props.changeCurrentDialog(DIALOG_SIGN_IN);
      return;
    }

    // Get sandbox name on successful login
    var sandboxName = data ? data.name : '';

    // Handle sandbox creation error
    if (!sandboxName) {
      this.props.changeDialogErrorMsg('Failed to sign in, try again...');
      this.props.changeCurrentDialog(DIALOG_SIGN_IN);
      return;
    }

    // Signing in
    this.props.changeDialogErrorMsg('');

    // Set sandbox name to be used in API requests
    this.props.changeSandboxName(sandboxName);
    this.setBasePaths();

    // Reset 
    this.props.changeNetworkFileSelected(DEFAULT_NO_NETWORK_FILE_SELECTED);

    // Display dialog while waiting for sandbox creation to complete
    this.props.changeCurrentDialog(DIALOG_SIGN_IN_WAIT);
    this.props.changeSignInStatus(STATUS_SIGNING_IN);
    this.startSandboxCreationTimer();

    // Start session keepalive timer
    this.startSessionKeepaliveTimer();
  }

  submitUserSignInInfo(creds) {
    var formParams = {
      'username': creds.userName,
      'password': creds.password
    };

    this.props.changeSignInUsername(creds.userName);
    this.meepAuthApi.loginUser(formParams, (error, data, response) => {
      this.loginCb(error, data, response);
    });
  }

  signInOAuth(provider) {
    // Stop timers & Set state to signing in
    this.stopTimers();
    this.props.changeCurrentDialog(DIALOG_SIGN_IN_WAIT);
    this.props.changeSignInStatus(STATUS_SIGNING_IN);

    // Navigate to OAuth login endpoint
    window.location.href = HOST_PATH + '/auth/v1/login?provider=' + provider + '&sbox=true';
  }

  deleteAppInstances() {
    var appInstances = this.props.appInstancesSelected;
    for (var i = 0; i < appInstances.length; i++) {
      this.meepApplicationsApi.applicationsAppInstanceIdDELETE(appInstances[i], null);
    }
    this.props.changeAppInstancesSelected([]);
  }

  createAppInstance(param) {
    var formParams = {
      'name': param.appName,
      'nodeName': param.mepName,
      'type': 'USER',
      'persist': false
    };
    this.meepApplicationsApi.applicationsPOST(formParams, (error, data, response) => {
      this.createAppInstanceCb(error, data, response);
    });
    this.props.changeCurrentDialog(null);
  }

  /**
     * Callback function to receive the result of the loginUser operation.
     * @callback module:api/AppInfo~createAppInstanceIdCallback
     * @param {String} error Error message, if any.
     * @param {appInstanceObject} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  createAppInstanceCb(error, data) {
    if (error) {
      var errMsg;
      switch (error.status) {
      case StatusCodes.UNAUTHORIZED:
        errMsg = 'Invalid credentials. Please try again...';
        break;
      case StatusCodes.SERVICE_UNAVAILABLE:
        errMsg = 'All EdgeNative - Connector sandboxes are currently in use. Please try again later...';
        break;
      default:
        errMsg = 'Login failure. Please try again...';
        break;
      }
      this.props.changeDialogErrorMsg(errMsg);
      this.props.changeCurrentDialog(DIALOG_CREATE_NEW_APP);
      return;
    }

    // Get sandbox name on successful login
    var appInstanceId = data ? data.id : '';

    // Handle applicationinstance creation error
    if (!appInstanceId) {
      this.props.changeDialogErrorMsg('Failed to create a new App Instance, try again...');
      this.props.changeCurrentDialog(DIALOG_CREATE_NEW_APP);
      return;
    }

    // clearing error
    this.props.changeDialogErrorMsg('');
  }

  updateAutomation(pauseButtonEnabled) {
    this.meepGisAutomationApi.setAutomationStateByName('MOVEMENT', !pauseButtonEnabled);
    this.meepGisAutomationApi.setAutomationStateByName('MOBILITY', !pauseButtonEnabled);
  }

  /**
     * Callback function to receive the result of the triggerWatchdog operation.
     * @callback module:api/AuthApi~triggerWatchdogCallback
     * @param {String} error Error message, if any.
     * @param data This operation does not return a value.
     * @param {String} response The complete HTTP response.
     */
  triggerWatchdogCb(error) {
    if (error) {
      return;
    }
  }

  sendKeepalive() {
    this.meepAuthApi.triggerWatchdog((error, data, response) => {
      this.triggerWatchdogCb(error, data, response);
    });
  }

  /**
     * Callback function to receive the result of the logout operation.
     * @callback module:api/AuthApi~logoutCallback
     * @param {String} error Error message, if any.
     * @param data This operation does not return a value.
     * @param {String} response The complete HTTP response.
     */
  logoutCb() {
    this.props.changeSignInStatus(STATUS_SIGNED_OUT);
    this.props.changeSandboxName('');
    this.updateScenario(null);
    this.props.changeTerminationInProgressCount(-1);
    this.props.changeActivationInProgressCount(-1);
    this.props.changeActivationInProgressScenarioName('');
    this.props.changeMap({ ueList: [], poaList: [], computeList: [] });

    if (this.props.page !== PAGE_HOME) {
      this.setMainContent(PAGE_HOME);
    }
  }

  logout() {
    this.stopSessionKeepaliveTimer();
    this.meepAuthApi.logout((error, data, response) => {
      this.logoutCb(error, data, response);
    });
  }

  /**
     * Callback function to receive the result of the getStates operation.
     * @callback module:api/MonEngineApi~getStatesCallback
     * @param {String} error Error message, if any.
     * @param {module:model/GetStates} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  getSandboxStateCb(error, data) {
    if (error) {
      // Silently ignore 503 errors during startup - services are still initializing
      if (error.status === StatusCodes.SERVICE_UNAVAILABLE) {
        return;
      }
      if (error.status === StatusCodes.UNAUTHORIZED) {
        this.stopSandboxCreationTimer();
        this.props.changeSignInStatus(STATUS_SIGNED_OUT);
      }
      this.props.changeCurrentDialog(null);
      return;
    }
    if (data) {
      for (var i in data.podStatus) {
        if (data.podStatus[i].logicalState !== 'Running') {
          return;
        }
      }
    }

    // all pods runnings, sandbox is ready
    this.stopSandboxCreationTimer();

    this.props.changeSignInStatus(STATUS_SIGNED_IN);
    this.props.changeCurrentPage(PAGE_SANDBOX);

    this.props.changeCurrentDialog(null);
    if (this.props.helpOnSignIn) {
      this.props.changeCurrentDialog(DIALOG_HELP_GETTING_STARTED);
    }
  }

  checkSandboxState() {
    // On page reload, restart sandbox creation timer to handle failure case
    if (!this.creationSandboxTimer) {
      this.startSandboxCreationTimer();
    }

    var sandboxName = this.props.sandboxName;
    if (sandboxName !== '') {
      var queryParams = {
        'type': 'core',
        'sandbox': sandboxName,
        'long': false
      };

      this.meepMonEngineApi.getStates(queryParams, (error, data, response) => {
        this.getSandboxStateCb(error, data, response);
      });
    }
  }

  /**
     * Callback function to receive the result of the getStates operation.
     * @callback module:api/MonEngineApi~getStatesCallback
     * @param {String} error Error message, if any.
     * @param {module:model/GetStates} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  getScenarioStateCb(error, data) {
    if (error) {
      return;
    }

    // Termination is complete when all pods are no longer in Running state
    if (data) {
      for (var i in data.podStatus) {
        if (data.podStatus[i].logicalState === 'Running') {
          return;
        }
      }
    }
    this.props.changeTerminationInProgressCount(-1);
  }

  checkScenarioState() {
    // Only check scenario state during termination
    if (this.props.terminationInProgressCount === -1) {
      return;
    }

    var sandboxName = this.props.sandboxName;
    if (sandboxName !== '') {
      var queryParams = {
        'type': 'scenario',
        'sandbox': sandboxName,
        'long': false
      };

      this.meepMonEngineApi.getStates(queryParams, (error, data, response) => {
        this.getScenarioStateCb(error, data, response);
      });
    }
  }

  /**
     * Callback function to receive the result of the getSandbox operation.
     * @callback module:api/SandboxControlApi~getSandboxCallback
     * @param {String} error Error message, if any.
     * @param {module:model/Sandbox} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  getSandboxCb(error) {
    if (error) {
      // Log user out if sandbox has been destroyed
      if (this.props.signInStatus === STATUS_SIGNED_IN) {
        if ((error.status === StatusCodes.UNAUTHORIZED) || (error.status === StatusCodes.NOT_FOUND)) {
          this.logout();
          this.props.changeCurrentDialog(DIALOG_SESSION_TERMINATED);
        } else if (this.sandboxErrorCount++ > SBOX_ERR_COUNT_MAX) {
          this.props.changeCurrentAlert(ALERT_DEGRADED_NETWORK);
        }
      }
      return;
    }

    // Reset error counter on success
    this.sandboxErrorCount = 0;

    // Clear bad network alert, if set
    if (this.props.currentAlert === ALERT_DEGRADED_NETWORK) {
      this.props.changeCurrentAlert(null);
    }
  }

  checkSignedIn() {
    // On page reload, restart session keepalive timer
    if (!this.sessionKeepaliveTimer) {
      this.startSessionKeepaliveTimer();
    }

    this.meepSandboxControlApi.getSandbox(this.props.sandboxName, (error, data, response) => {
      this.getSandboxCb(error, data, response);
    });
  }

  /**
     * Callback function to receive the result of the getActiveScenario operation.
     * @callback module:api/ScenarioExecutionApi~getActiveScenarioCallback
     * @param {String} error Error message, if any.
     * @param {module:model/Scenario} data The data returned by the service call.
     */
  getActiveScenarioCb(error, data) {
    if (error !== null) {
      if (error.status === 404) {
        this.updateScenario(null);
        this.props.changeApiTable(null);
        this.props.changeApiDetailedData(null);
        this.props.changeAppInstanceTable([]);
        this.props.changeSvcTable([]);
      }
      return;
    }

    // Check if response indicates no scenario is active (new format)
    if (data && data.status === 'inactive') {
      this.updateScenario(null);
      this.props.changeApiTable(null);
      this.props.changeApiDetailedData(null);
      this.props.changeAppInstanceTable([]);
      this.props.changeSvcTable([]);
      return;
    }

    // Validate scenario
    if (data && !data.deployment) {
      return;
    }

    // Store & Process deployed scenario
    if (this.props.terminationInProgressCount !== -1) {
      // Ignore active scenario polling while termination is in progress
      return;
    }

    if (this.props.activationInProgressCount === -1) {
      this.updateScenario(data);
    } else {
      if (this.props.activationInProgressScenarioName === data.name) {
        this.updateScenario(data);
        this.props.changeActivationInProgressCount(-1);
      }
    }
  }

  // Change & process scenario
  updateScenario(scenario) {
    // Change scenario state
    this.props.changeScenario(scenario);

    // Parse Scenario & update table
    var parsedScenario = parseScenario(scenario);
    this.props.changeTable(parsedScenario ? parsedScenario.table : null);
  }

  // Refresh Active scenario
  refreshScenario() {
    this.meepActiveScenarioApi.getActiveScenario({ minimize: 'true' }, (error, data) =>
      this.getActiveScenarioCb(error, data)
    );
  }

  syncUeButtons(mapUeList) {
    var nbHv = 0, nbLv = 0, nbZv = 0;
    var found = false;
    var i, j;

    if (!mapUeList || this.props.updateUeInProgressCount !== -1) {
      return;
    }

    for (j = 0; j < mapUeList.length; j++) {
      found = false;
      for (i = 0; i < this.props.highVelocityUeList.length; i++) {
        if (this.props.highVelocityUeList[i].name === mapUeList[j].assetName) {
          nbHv++;
          found = true;
        }
      }
      if (!found) {
        for (i = 0; i < this.props.lowVelocityUeList.length; i++) {
          if (this.props.lowVelocityUeList[i].name === mapUeList[j].assetName) {
            nbLv++;
            found = true;
          }
        }
      }
      if (!found) {
        for (i = 0; i < this.props.stationaryUeList.length; i++) {
          if (this.props.stationaryUeList[i].name === mapUeList[j].assetName) {
            nbZv++;
          }
        }
      }
    }
    this.props.changeNbLowVelocityUe(nbLv);
    this.props.changeNbHighVelocityUe(nbHv);
    this.props.changeNbStationaryUe(nbZv);
  }

  /**
     * Callback function to receive the result of the getAssetData operation.
     * @callback module:api/GeospatialDataApi~getAssetDataCallback
     * @param {String} error Error message, if any.
     * @param {module:model/GeoDataAssetList} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  getAssetDataCb(error, data) {
    if (error !== null) {
      return;
    }

    var ueList = [];
    var poaList = [];
    var computeList = [];

    // Extract assets by type
    if (data.geoDataAssets) {
      _.forEach(data.geoDataAssets, asset => {
        switch (asset.assetType) {
        case 'UE':
          ueList.push(asset);
          break;
        case 'POA':
          poaList.push(asset);
          break;
        case 'COMPUTE':
          computeList.push(asset);
          break;
        default:
          break;
        }
      });
    }

    // Use previous UE paths using paths from previous UE list (if not updated)
    if (!this.refreshUePaths) {
      var prevUeList = this.props.map.ueList;
      for (var i = 0; i < ueList.length; i++) {
        var found = false;
        for (var j = 0; j < prevUeList.length; j++) {
          if (ueList[i].assetName === prevUeList[j].assetName) {
            ueList[i].path = prevUeList[j].path;
            found = true;
            break;
          }
        }
        // If new UE in list, refresh UE paths on next request
        if (!found) {
          this.uePathUpdateRequired = true;
        }
      }
    }
    this.refreshUePaths = false;

    // Update asset map
    var assetMap = {
      ueList: _.sortBy(ueList, ['assetName']),
      poaList: _.sortBy(poaList, ['assetName']),
      computeList: _.sortBy(computeList, ['assetName'])
    };
    this.props.changeMap(assetMap);

    // Sync UE button state with backend 
    this.syncUeButtons(ueList);
  }

  // Refresh Map
  refreshMap() {
    // If UE cound has increased, include UE paths in the next request
    if (this.uePathUpdateRequired) {
      this.refreshUePaths = true;
      this.uePathUpdateRequired = false;
    }
    this.meepGeoDataApi.getAssetData({ excludePath: !this.refreshUePaths }, (error, data) =>
      this.getAssetDataCb(error, data)
    );
  }

  /**
     * Callback function to receive the result of the get automation (movement) operation.
     * @callback module:api/ScenarioAutomationApi~automationCallback
     * @param {String} error Error message, if any.
     */
  getAutomationCb(error, data) {
    if (error) {
      return;
    }

    // Make sure are ready to update state
    if (this.props.updateAutomationInProgressCount !== -1) {
      return;
    }

    // Update pause button state
    var paused = !data.active;
    if (this.props.pauseButton !== paused) {
      this.props.changePauseButton(paused);
    }
  }

  refreshAutomation() {
    this.meepGisAutomationApi.getAutomationStateByName('MOVEMENT', (error, data, response) =>
      this.getAutomationCb(error, data, response)
    );
  }

  //get oldest data in the data that matches an entry in oldData
  mergeApiTableData(...arrays) {
    let jointArray = [];
    let resultArray = [];
    arrays.forEach(array => {
      if (array) {
        jointArray = [...array, ...jointArray];
      }
    });

    var len = jointArray.length;
    var assoc = {};

    while (len--) {
      var item = jointArray[len];
      var itemId = item.id + '-' + item.loggerName + '-' + item.time;
      if (!assoc[itemId]) {
        resultArray.unshift(item);
        assoc[itemId] = true;
      }
    }
    return resultArray.slice(0, apiTableMaxSize);
  }

  /**
     * Callback function to receive the result of the postHttpQuery operation.
     * @callback module:api/MetricsApi~postHttpQuery
     * @param {String} error Error message, if any.
     * @param {module:model/HttpQuery} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  postHttpQueryCb(error, data) {
    if (error !== null) {
      return;
    }

    // only update if values present, otherwise means no changes
    if (data) {
      if (this.props.apiTable !== null) {
        data.values = this.mergeApiTableData(this.props.apiTable.values, data.values);
      }
      this.props.changeApiTable(data);
    }
  }

  refreshMetricsTable() {
    // If polling just started, refresh entire API console list
    if (this.refreshApiConsole) {
      metricsQuery.scope.duration = '1d';
      this.refreshApiConsole = false;
    } else {
      metricsQuery.scope.duration = '2s';
    }
    this.meepMetricsEngineApi.postHttpQuery(metricsQuery, (error, data, response) => {
      this.postHttpQueryCb(error, data, response);
    });
  }

  /**
     * Callback function to receive the result of the servicesGET operation.
     * @callback module:api/ServicesApi~servicesGETCallback
     * @param {String} error Error message, if any.
     * @param {Array.<module:model/ServiceInfo>} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  getServicesCb(error, data) {
    if (error !== null) {
      this.props.changeSvcTable([]);
      return;
    }

    // Make sure are ready to update state
    if (this.props.updateAppInstancesInProgressCount !== -1) {
      return;
    }

    // Update App Instance table only if data is different 
    var services = data ? data : [];
    const isArrayEqual = (x, y) => _.isEmpty(_.xorWith(x, y, _.isEqual));
    if (!isArrayEqual(this.props.svcTable, services)) {
      this.props.changeSvcTable(services);

      // Update Edge App states
      this.refreshEdgeApps(this.props.appInstanceTable, services);
    }
  }

  refreshServices() {
    this.meepServicesApi.servicesGET(null, (error, data, response) => {
      this.getServicesCb(error, data, response);
    });
  }

  /**
     * Callback function to receive the result of the postHttpQuery operation.
     * @callback module:api/AppsApi~applicationsGET
     * @param {String} error Error message, if any.
     * @param {module:model/ApplicationInfo} data The data returned by the service call.
     * @param {String} response The complete HTTP response.
     */
  getAppInstancesCb(error, data) {
    if (error !== null) {
      this.props.changeAppInstanceTable([]);
      return;
    }

    // Make sure are ready to update state
    if (this.props.updateAppInstancesInProgressCount !== -1) {
      return;
    }

    // Update App Instance table only if data is different 
    var appInstances = data ? data : [];
    const isArrayEqual = (x, y) => _.isEmpty(_.xorWith(x, y, _.isEqual));
    if (!isArrayEqual(this.props.appInstanceTable, appInstances)) {
      this.props.changeAppInstanceTable(appInstances);

      // Update Edge App states
      this.refreshEdgeApps(appInstances, this.props.svcTable);
    }
  }

  refreshAppInstancesTable() {
    this.meepApplicationsApi.applicationsGET(null, (error, data, response) => {
      this.getAppInstancesCb(error, data, response);
    });
  }

  getAppInstance(id, appInstances) {
    if (appInstances) {
      for (var i = 0; i < appInstances.length; i++) {
        let appInstance = appInstances[i];
        if (appInstance.id === id) {
          return appInstance;
        }
      }
    }
    return null;
  }

  getSvcInstance(appId, services) {
    if (services) {
      for (var i = 0; i < services.length; i++) {
        let service = services[i];
        if (service.appId === appId) {
          return service;
        }
      }
    }
    return null;
  }

  refreshEdgeApps(appInstances, services) {
    if (!this.props.edgeApps) {
      return;
    }

    let edgeApps = [];
    let updateRequired = false;

    // Loop through edge apps
    for (var i = 0; i < this.props.edgeApps.length; i++) {
      // Get a copy of the edge app
      let edgeApp = updateObject(this.props.edgeApps[i], {});

      // Ignore mec011 & enable/disable in progress
      let isMec011 = edgeApp.pseudoName.includes('011');
      if (isMec011 || edgeApp.enableInProgressCount > -1 || edgeApp.disableInProgressCount > -1) {
        edgeApps.push(edgeApp);
        continue;
      }

      // Find matching app instance, if any
      let appInstance = this.getAppInstance(edgeApp.id, appInstances);
      let svcInstance = this.getSvcInstance(edgeApp.id, services);

      // Update app instance ID when available
      if (appInstance && !edgeApp.appId) {
        edgeApp.appId = edgeApp.id;
        updateRequired = true;
      }

      // Update service state
      if (!appInstance || !svcInstance) {
        if (edgeApp.enabled) {
          edgeApp.enabled = false;
          edgeApp.appId = '';
          edgeApp.svcId = '';
          updateRequired = true;
        }
      } else {
        if (!edgeApp.enabled) {
          edgeApp.instance = appInstance.name;
          edgeApp.svcId = svcInstance.id;
          edgeApp.enabled = true;
          updateRequired = true;
        }
      }

      // Add edge app to list
      edgeApps.push(edgeApp);
    }

    // Update state if required
    if (updateRequired) {
      this.props.changeEdgeAppList(edgeApps);
    }
  }

  decrementEdgeApps(appInstances, services) {
    if (!this.props.edgeApps) {
      return;
    }

    let edgeApps = [];
    let updateRequired = false;

    // Loop through edge apps
    for (var i = 0; i < this.props.edgeApps.length; i++) {
      // Get a copy of the edge app
      let edgeApp = updateObject(this.props.edgeApps[i], {});

      // Ignore mec011
      let isMec011 = edgeApp.pseudoName.includes('011');
      if (isMec011) {
        edgeApps.push(edgeApp);
        continue;
      }

      // Decrement progress counters if enable/disable in progress
      if (edgeApp.enableInProgressCount > -1) {
        edgeApp.enableInProgressCount--;
        // Enable controls immediately after operation is complete
        if (edgeApp.enableInProgressCount < (EDGE_APP_ENABLE_COUNT_MAX - BLACKOUT_TIME)) {
          let appInstance = this.getAppInstance(edgeApp.id, appInstances);
          let svcInstance = this.getSvcInstance(edgeApp.id, services);
          if (appInstance && !edgeApp.appId) {
            edgeApp.appId = edgeApp.id;
          }
          if (appInstance && svcInstance) {
            edgeApp.instance = appInstance.name;
            edgeApp.appId = edgeApp.id;
            edgeApp.svcId = svcInstance.id;
            edgeApp.enabled = true;
            edgeApp.enableInProgressCount = -1;
          }
        }
        updateRequired = true;
      } else if (edgeApp.disableInProgressCount > -1) {
        edgeApp.disableInProgressCount--;
        // Enable controls immediately after operation is complete
        if (edgeApp.disableInProgressCount < (EDGE_APP_DISABLE_COUNT_MAX - BLACKOUT_TIME)) {
          let appInstance = this.getAppInstance(edgeApp.id, appInstances);
          let svcInstance = this.getSvcInstance(edgeApp.id, services);
          if (!appInstance || !svcInstance) {
            edgeApp.enabled = false;
            edgeApp.appId = '';
            edgeApp.svcId = '';
            edgeApp.disableInProgressCount = -1;
          }
        }
        updateRequired = true;
      }

      // Add edge app to list
      edgeApps.push(edgeApp);
    }

    // Update state if required
    if (updateRequired) {
      this.props.changeEdgeAppList(edgeApps);
    }
  }

  decrementProgressCounters() {
    // Update counters
    if (this.props.terminationInProgressCount !== -1) {
      this.props.changeTerminationInProgressCount(this.props.terminationInProgressCount - 1);
    }
    if (this.props.activationInProgressCount !== -1) {
      this.props.changeActivationInProgressCount(this.props.activationInProgressCount - 1);
    }
    if (this.props.updateUeInProgressCount !== -1) {
      this.props.changeUpdateUeInProgressCount(this.props.updateUeInProgressCount - 1);
    }
    if (this.props.updateAutomationInProgressCount !== -1) {
      this.props.changeUpdateAutomationInProgressCount(this.props.updateAutomationInProgressCount - 1);
    }
    if (this.props.updateAppInstancesInProgressCount !== -1) {
      this.props.changeUpdateAppInstancesInProgressCount(this.props.updateAppInstancesInProgressCount - 1);
    }

    // Edge App counters
    this.decrementEdgeApps(this.props.appInstanceTable, this.props.svcTable);
  }

  renderPage() {
    switch (this.props.page) {
    case PAGE_HOME:
      return (
        <HomePageContainer
          style={{ width: '100%', height: '100%' }}
        />
      );

    case PAGE_SANDBOX:
      return (
        <SandboxPageContainer
          style={{ width: '100%' }}
          scenarioApi={this.meepScenarioConfigurationApi}
          activateApi={this.meepActiveScenarioApi}
          eventsApi={this.meepEventsApi}
          metricsApi={this.meepMetricsEngineApi}
          automationApi={this.meepGisAutomationApi}
          applicationsApi={this.meepApplicationsApi}
        />
      );

    case PAGE_DASHBOARD:
      return (
        <DashboardPageContainer
          style={{ width: '100%' }}
        />
      );

    default:
      return null;
    }
  }

  renderDialogs() {
    return (
      <>

        {/* <SignInDialog
          title='Sign In'
          open={this.props.currentDialog === DIALOG_SIGN_IN}
          onClose={() => {
            this.closeDialog();
          }}
          onSubmit={creds => this.submitUserSignInInfo(creds)}
          errorMsg={this.props.dialogErrorMsg}
        /> */}
        <SignInOAuthDialog
          title='Sign in with'
          open={this.props.currentDialog === DIALOG_SIGN_IN}
          onSignIn={provider => this.signInOAuth(provider)}
          onClose={() => {
            this.closeDialog();
          }}
          onShowSslHelp={() => {
            this.closeDialog();
            setTimeout(() => {
              this.props.changeCurrentDialog(DIALOG_SSL_TRUST_ERROR);
            }, 500);
          }}
        />
        <SignInWaitDialog
          title='Initializing sandbox'
          open={this.props.currentDialog === DIALOG_SIGN_IN_WAIT}
          onClose={() => {
            this.closeDialog();
          }}
        />
        <SessionTerminatedDialog
          title='Session ended'
          open={this.props.currentDialog === DIALOG_SESSION_TERMINATED}
          onClose={() => {
            this.closeDialog();
          }}
        />
        <HelpGettingStartedDialog
          title='Getting started'
          open={this.props.currentDialog === DIALOG_HELP_GETTING_STARTED}
          onClose={() => {
            this.closeDialog();
          }}
        />
        <ConfirmDeleteAppDialog
          title='Delete MEC Application instance ID'
          open={this.props.currentDialog === DIALOG_CONFIRM_DELETE_APP}
          onSubmit={() => {
            this.deleteAppInstances();
          }}
          onClose={() => {
            this.closeDialog();
          }}
        />
        <CreateNewAppDialog
          title='Create MEC Application Instance ID'
          open={this.props.currentDialog === DIALOG_CREATE_NEW_APP}
          onSubmit={param => this.createAppInstance(param)}
          onClose={() => {
            this.closeDialog();
          }}
          mepNames={this.props.mepList}
        />
        <VersionDialog
          title='EdgeNative - Connector Versions'
          open={this.props.currentDialog === DIALOG_VERSION}
          onClose={() => {
            this.closeDialog();
          }}
        />
        <SslTrustErrorDialog
          title='Connection SSL/Cookie Security Alert'
          open={this.props.currentDialog === DIALOG_SSL_TRUST_ERROR}
          onClose={(closeOnSubmit) => {
            this.closeDialog();
            if (closeOnSubmit === true) {
              setTimeout(() => {
                this.props.changeCurrentDialog(DIALOG_SIGN_IN);
              }, 500);
            }
          }}
        />
      </>
    );
  }

  render() {
    let width = window.innerWidth;
    let height = window.innerHeight;

    if ((width >= MIN_SCREEN_WIDTH) && (height >= MIN_SCREEN_HEIGHT)) {
      return (
        <div>

          {this.renderDialogs()}

          <TopBarContainer
            onClickSignIn={() => this.signInProcedure()}
          />
          <div style={{ height: 32 }} />

          <TopAppBarFixedAdjust dense />

          {this.renderPage()}

          <Footer />
        </div>
      );
    } else {
      return (
        <div style={{ margin: 20 }}>
          <h1>EdgeNative - Connector</h1>
          <h3>Unfortunately, EdgeNative - Connector does not look great with a resolution of {width} x {height}</h3>
          <p>
                        Please zoom out and refresh your browser or load EdgeNative - Connector from a browser with a
                        minimum width of <b>{MIN_SCREEN_WIDTH}</b> pixels.
          </p>
          <p>NOTE: keyboard zoom shortcut is <i>ctrl+- (minus)</i></p>
        </div>
      );
    }
  }
}

const mapStateToProps = state => {
  return {
    currentDialog: state.ui.currentDialog,
    currentAlert: state.ui.currentAlert,
    page: state.ui.page,
    helpOnSignIn: state.ui.helpOnSignIn,
    sandboxName: state.ui.sandboxName,
    mainDrawerOpen: state.ui.mainDrawerOpen,
    signInStatus: state.ui.signInStatus,
    signInUsername: state.ui.signInUsername,
    dialogErrorMsg: state.ui.dialogErrorMsg,
    networkFiles: state.ui.networkFiles,
    networkFileSelected: state.ui.networkFileSelected,
    apiTable: state.sbox.apiTable.data,
    highVelocityUeList: state.ui.highVelocityUeList,
    lowVelocityUeList: state.ui.lowVelocityUeList,
    stationaryUeList: state.ui.stationaryUeList,
    nbLowVelocityUe: state.ui.nbLowVelocityUe,
    nbHighVelocityUe: state.ui.nbHighVelocityUe,
    mepList: state.ui.mepList,
    edgeApps: state.ui.edgeApps,
    appInstanceTable: state.sbox.appInstanceTable.data,
    appInstancesSelected: state.ui.appInstancesSelected,
    svcTable: state.sbox.svcTable.data,
    updateUeInProgressCount: state.ui.updateUeInProgressCount,
    updateAppInstancesInProgressCount: state.ui.updateAppInstancesInProgressCount,
    terminationInProgressCount: state.ui.terminationInProgressCount,
    activationInProgressCount: state.ui.activationInProgressCount,
    activationInProgressScenarioName: state.ui.activationInProgressScenarioName,
    map: state.sbox.map,
    scenario: state.sbox.scenario
  };
};

const mapDispatchToProps = dispatch => {
  return {
    // UI
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type)),
    changeDialogErrorMsg: msg => dispatch(uiChangeDialogErrorMsg(msg)),
    changeCurrentAlert: alert => dispatch(uiChangeCurrentAlert(alert)),
    changeCurrentPage: page => dispatch(uiChangeCurrentPage(page)),
    changeHelpOnSignIn: check => dispatch(uiChangeHelpOnSignIn(check)),
    changeSandboxName: name => dispatch(uiChangeSandboxName(name)),
    changeNetworkFileSelected: name => dispatch(uiSandboxChangeNetworkFileSelected(name)),
    changeNetworkInfo: value => dispatch(uiSandboxChangeNetworkInfo(value)),
    changeNbStationaryUe: value => dispatch(uiSandboxChangeNbStationaryUe(value)),
    changeNbLowVelocityUe: value => dispatch(uiSandboxChangeNbLowVelocityUe(value)),
    changeNbHighVelocityUe: value => dispatch(uiSandboxChangeNbHighVelocityUe(value)),
    changeStationaryUeList: list => dispatch(uiSandboxChangeStationaryUeList(list)),
    changeLowVelocityUeList: list => dispatch(uiSandboxChangeLowVelocityUeList(list)),
    changeHighVelocityUeList: list => dispatch(uiSandboxChangeHighVelocityUeList(list)),
    changeMepList: list => dispatch(uiSandboxChangeMepList(list)),
    changeEdgeAppList: list => dispatch(uiSandboxChangeEdgeAppList(list)),
    changeAppInstancesSelected: list => dispatch(uiSandboxChangeAppInstancesSelected(list)),
    changePauseButton: checked => dispatch(uiSandboxChangePauseButton(checked)),
    changeApiDetailedData: row => dispatch(uiSandboxChangeApiDetailedData(row)),
    changeSignInStatus: status => dispatch(uiChangeSignInStatus(status)),
    changeSignInUsername: name => dispatch(uiChangeSignInUsername(name)),
    changeSignInUserRole: role => dispatch(uiChangeSignInUserRole(role)),
    // SBOX
    changeScenario: scenario => dispatch(sboxChangeScenario(scenario)),
    changeTable: table => dispatch(sboxChangeTable(table)),
    changeMap: list => dispatch(sboxChangeMap(list)),
    changeMapUeList: list => dispatch(sboxChangeMapUeList(list)),
    changeMapPoaList: list => dispatch(sboxChangeMapPoaList(list)),
    changeMapComputeList: list => dispatch(sboxChangeMapComputeList(list)),
    changeApiTable: value => dispatch(sboxChangeApiTable(value)),
    changeAppInstanceTable: value => dispatch(sboxChangeAppInstanceTable(value)),
    changeSvcTable: value => dispatch(sboxChangeSvcTable(value)),
    changeUpdateUeInProgressCount: count => dispatch(uiSandboxChangeUpdateUeInProgressCount(count)),
    changeUpdateAutomationInProgressCount: count => dispatch(uiSandboxChangeUpdateAutomationInProgressCount(count)),
    changeUpdateAppInstancesInProgressCount: count => dispatch(uiSandboxChangeUpdateAppInstancesInProgressCount(count)),
    changeTerminationInProgressCount: count => dispatch(uiSandboxChangeTerminationInProgressCount(count)),
    changeActivationInProgressCount: count => dispatch(uiSandboxChangeActivationInProgressCount(count)),
    changeActivationInProgressScenarioName: name => dispatch(uiSandboxChangeActivationInProgressScenarioName(name))
  };
};

const ConnectedAppContainer = connect(
  mapStateToProps,
  mapDispatchToProps
)(AppContainer);

export default ConnectedAppContainer;
