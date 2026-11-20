/*
 * Copyright (c) 2022-2025 ETSI.  All rights reserved.
 */

// Version
export const EDGENATIVE_CONNECTOR_VERSION = 'v1.11';

// Host
export const HOST_PATH = location.origin;

// Pages
export const PAGE_HOME = 'HOME';
export const PAGE_SANDBOX = 'SANDBOX';
export const PAGE_DASHBOARD = 'DASHBOARD';

// Networks
export const DEFAULT_NO_NETWORK_FILE_SELECTED = 'Select a network';

// Minimum screen resolution
export const MIN_SCREEN_WIDTH = 840;
export const MIN_SCREEN_HEIGHT = 0;

// User Application Instances
export const MAX_NB_USER_APPS = 10;

// URLs
export const WIKI_HELP_URL = 'https://mecwiki.etsi.org/index.php?title=MEC_Sandbox_Help';
export const WIKI_DISCUSSION_BOARD_URL = 'https://mecwiki.etsi.org/index.php?title=MEC_Sandbox_Help#Discussion_Board_.28Slack_channels.29';
export const WIKI_REPORT_ISSUE_URL = 'https://mecwiki.etsi.org/index.php?title=MEC_Sandbox_Help#Reporting_Issues';
export const ETSI_MEC_URL = 'https://www.etsi.org/technologies/multi-access-edge-computing';
export const ETSI_PORTAL_URL = 'https://portal.etsi.org/tb.aspx?tbid=826&SubTB=826';
export const ETSI_FORGE_URL = 'https://forge.etsi.org';
export const ETSI_FORGE_MEC_URL = 'https://forge.etsi.org/rep/mec';
export const ETSI_PRIVACY_URL = 'https://forge.etsi.org/mec-sandbox-privacy';
export const ETSI_TERMS_OF_USE_URL = 'https://forge.etsi.org/mec-sandbox-terms-and-conditions';
export const ETSI_CONTACT_URL = 'https://www.etsi.org/contact';
export const EDGENATIVE_CONNECTOR_OVERVIEW_PDF_URL = 'https://portal.etsi.org/Portals/0/TBpages/MEC/Docs/ETSI-MEC-Public-Overview_Generic.pdf';
export const EDGENATIVE_CONNECTOR_WEBINAR_URL = 'https://mecwiki.etsi.org/index.php?title=MEC_Sandbox_Help#MEC_Sandbox_Concepts_.26_Usage';

export const DASHBOARD_EDGENATIVE_CONNECTOR_URL = HOST_PATH + '/grafana/d/mec-sandbox/mec-sandbox?orgId=1&from=now-24h&to=now&refresh=15s&kiosk=tv&theme=light';
export const DASHBOARD_PLATFORM_ADVANTEDGE_URL = HOST_PATH + '/grafana/d/platform-advantedge/platform-advantedge?orgId=1&from=now-24h&to=now&refresh=15s&kiosk=tv&theme=light';
export const DASHBOARD_PLATFORM_K8S_URL = HOST_PATH + '/grafana/d/platform-k8s/platform-k8s?orgId=1&from=now-24h&to=now&refresh=30s&kiosk=tv&theme=light';
export const DASHBOARD_RUNTIME_ENVIRONMENT_NODE_URL = HOST_PATH + '/grafana/d/runtime-environment-node/runtime-environment-node?orgId=1&from=now-24h&to=now&refresh=30s&kiosk=tv&theme=light';
export const DASHBOARD_RUNTIME_ENVIRONMENT_K8S_URL = HOST_PATH + '/grafana/d/runtime-environment-k8s-cluster/runtime-environment-k8s-cluster?orgId=1&from=now-24h&to=now&refresh=30s&kiosk=tv&theme=light';

// IDs
export const SANDBOX_NETWORK_FILES = 'sandbox-network-files';
export const SANDBOX_MEC_API = 'sandbox-mec-api';

// Dialog IDs
export const DLG_DETAILED_DATA = 'dlg-detailed-data';
export const DLG_SIGN_IN = 'dlg-sign-in';
export const DLG_CONFIRM_DELETE_APP = 'dlg-confirm-delete-app';
export const DLG_CREATE_NEW_APP = 'dlg-create-new-app';

// Dialog Types
export const DIALOG_DETAILED_DATA = 'DIALOG_DETAILED_DATA';
export const DIALOG_SIGN_IN = 'DIALOG_SIGN_IN';
export const DIALOG_SIGN_IN_WAIT = 'DIALOG_SIGN_IN_WAIT';
export const DIALOG_SESSION_TERMINATED = 'DIALOG_SESSION_TERMINATED';
export const DIALOG_HELP_GETTING_STARTED = 'DIALOG_HELP_GETTING_STARTED';
export const DIALOG_CONFIRM_DELETE_APP = 'DIALOG_CONFIRM_DELETE_APP';
export const DIALOG_CREATE_NEW_APP = 'DIALOG_CREATE_NEW_APP';
export const DIALOG_VERSION = 'DIALOG_VERSION';


// Alert Types
export const ALERT_DEGRADED_NETWORK = 'ALERT_DEGRADED_NETWORK';

// Trivia
export const NO_SCENARIO_NAME = 'NO_SCENARIO_NAME_12Q(*&HGHG___--9098';

// Sign In Status
export const STATUS_SIGNED_IN = 'SIGNED-IN';
export const STATUS_SIGNING_IN = 'SIGNING-IN';
export const STATUS_SIGNED_OUT = 'SIGNED-OUT';

// OAuth Providers
export const OAUTH_PROVIDER_GITHUB = 'github';
export const OAUTH_PROVIDER_GITLAB = 'gitlab';

// Configuration pane defaults
export const DEFAULT_VELOCITY_THRESHOLD = 10;

// API pane defaults
export const EDGE_APP_ENABLE_COUNT_MAX = 20;
export const EDGE_APP_DISABLE_COUNT_MAX = 5;
export const BLACKOUT_TIME = 3;

// Scenario element types
export const DOMAIN_TYPE_STR = 'OPERATOR';
export const DOMAIN_CELL_TYPE_STR = 'OPERATOR-CELLULAR';
export const PUBLIC_DOMAIN_TYPE_STR = 'PUBLIC';
export const ZONE_TYPE_STR = 'ZONE';
export const COMMON_ZONE_TYPE_STR = 'COMMON';
export const NL_TYPE_STR = 'POA';
export const POA_TYPE_STR = 'POA';
export const POA_4G_TYPE_STR = 'POA-4G';
export const POA_5G_TYPE_STR = 'POA-5G';
export const POA_WIFI_TYPE_STR = 'POA-WIFI';
export const DEFAULT_NL_TYPE_STR = 'DEFAULT';
export const UE_TYPE_STR = 'UE';
export const FOG_TYPE_STR = 'FOG';
export const EDGE_TYPE_STR = 'EDGE';
export const CN_TYPE_STR = 'CN';
export const DC_TYPE_STR = 'DC';
export const MEC_SVC_TYPE_STR = 'MEC-SVC';
export const UE_APP_TYPE_STR = 'UE-APP';
export const EDGE_APP_TYPE_STR = 'EDGE-APP';
export const CLOUD_APP_TYPE_STR = 'CLOUD-APP';

export const ELEMENT_TYPE_SCENARIO = 'SCENARIO';
export const ELEMENT_TYPE_OPERATOR = 'OPERATOR';
export const ELEMENT_TYPE_OPERATOR_GENERIC = 'OPERATOR GENERIC';
export const ELEMENT_TYPE_OPERATOR_CELL = 'OPERATOR CELLULAR';
export const ELEMENT_TYPE_ZONE = 'ZONE';
export const ELEMENT_TYPE_POA = 'POA';
export const ELEMENT_TYPE_POA_GENERIC = 'POA GENERIC';
export const ELEMENT_TYPE_POA_4G = 'POA CELLULAR 4G';
export const ELEMENT_TYPE_POA_5G = 'POA CELLULAR 5G';
export const ELEMENT_TYPE_POA_WIFI = 'POA WIFI';
export const ELEMENT_TYPE_DC = 'DISTANT CLOUD';
export const ELEMENT_TYPE_CN = 'CORE NETWORK';
export const ELEMENT_TYPE_EDGE = 'EDGE';
export const ELEMENT_TYPE_FOG = 'FOG';
export const ELEMENT_TYPE_UE = 'UE';
export const ELEMENT_TYPE_MECSVC = 'MEC SERVICE';
export const ELEMENT_TYPE_UE_APP = 'UE APPLICATION';
export const ELEMENT_TYPE_EDGE_APP = 'EDGE APPLICATION';
export const ELEMENT_TYPE_CLOUD_APP = 'CLOUD APPLICATION';

// MEC Services configuration
export const MEC_SERVICES = [
  { prefix: 'meep-app-enablement', code: '011', name: 'App Enablement' },
  { prefix: 'meep-rnis', code: '012', name: 'RNIS' },
  { prefix: 'meep-loc-serv', code: '013', name: 'Location' },
  { prefix: 'meep-tm', code: '015', name: 'Traffic Management' },
  { prefix: 'meep-dai', code: '016', name: 'Device Application Interface' },
  { prefix: 'meep-ams', code: '021', name: 'Application Mobility' },
  { prefix: 'meep-wais', code: '028', name: 'WLAN Access Information' },
  { prefix: 'meep-vis', code: '030', name: 'V2X Information' },
  { prefix: 'meep-iot', code: '033', name: 'IoT API' },
  { prefix: 'meep-federation', code: '040', name: 'Federation API' },
  { prefix: 'meep-sss', code: '046', name: 'SSS API' }
];
