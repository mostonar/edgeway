/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import BasicDialog from './basic-dialog';
import { Typography } from '@rmwc/typography';

class VersionDialog extends Component {
  constructor(props) {
    super(props);
  }

  render() {
    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onSubmit={this.props.onClose}
        onClose={this.props.onClose}
        submitLabel = {'Ok'}
      >
        <div style={styles.text}>

          <Typography theme="primary" use="subtitle1"><p><b>v1.11 &bull;</b> 2026-02-25</p></Typography>
          <Typography use="body1">
            <p>AdvantEDGE baseline upgrade to v1.11.0</p>
            STF678 Final:
            <ul>
              <li>MEC Federation Service API support</li>
              <li>EdgeNative - Connector API for command line support</li>
              <li>Support of Edge Native Connector (3GPP TS 29.222: 3rd Generation Partnership Project; Technical Specification Group Core Network and Terminals; Common API Framework for 3GPP Northbound APIs)</li>
              <li>General maintenance fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.9 &bull;</b> 2023-02-22</p></Typography>
          <Typography use="body1">
            <p>AdvantEDGE baseline upgrade to v1.9.0</p>
            STF625 Final:
            <ul>
              <li>V2X messsage distribution support</li>
              <li>General maintenance fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.8 &bull;</b> 2022-09-29</p></Typography>
          <Typography use="body1">
            STF625 update 2:
            <ul>
              <li>Addition of the BWM API (Bandwidth Management Service) v2.2.1</li>
              <li>Addition of the MTS API (Multi-access Traffic Steering Service) v2.2.1</li>
              <li>Addition of the DAI API (Device Application Interface Service) v2.2.1</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.7 &bull;</b> 2022-06-20</p></Typography>
          <Typography use="body1">
            STF625 update:
            <ul>
              <li>Addition of the VIS API (V2X Service) v2.2.1</li>
              <li>Upgradation of MEC011, MEC012, MEC013 and MEC021 to v2.2.1</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.6 &bull;</b> 2021-12-16</p></Typography>
          <Typography use="body1">
            Final STF599 update:
            <ul>
              <li>AdvantEDGE baseline upgrade to v1.8.1</li>
              <li>General maintenance fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.5.3 (beta) &bull;</b> 2021-11-22</p></Typography>
          <Typography use="body1">
            Maintenance Release:
            <ul>
              <li>Service IDs displayed in frontend</li>
              <li>Garbage collector for internal databases</li>
              <li>User-persistent sandbox names</li>
              <li>MEC Service bug fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.5.2 (beta) &bull;</b> 2021-11-08</p></Typography>
          <Typography use="body1">
            MEC028 WebSockets:
            <ul>
              <li>MEC028 subscriptions - support for MEC009 WebSocket fallback pattern</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.5.1 (beta) &bull;</b> 2021-10-30</p></Typography>
          <Typography use="body1">
            MEC API Maintenance:
            <ul>
              <li>MEC Service bug fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.5 (beta) &bull;</b> 2021-09-30</p></Typography>
          <Typography use="body1">
            New MEC APIs:
            <ul>
              <li>MEC011 - Edge Platform Application Enablement</li>
              <li>MEC021 - Application Mobility Service</li>
              <li>Dual MEP network to showcase new APIs</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.4.1 &bull;</b> 2021-08-06</p></Typography>
          <Typography use="body1">
            MEC Service Maintenance:
            <ul>
              <li>MEC Service bug fixes</li>
              <li>Beta tags removed from API endpoints</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.4 (beta) &bull;</b> 2021-07-20</p></Typography>
          <Typography use="body1">
            MEC Service Enhancements:
            <ul>
              <li>MEC012, MEC013 & MEC028 support for all API endpoints</li>
              <li>MEC Service bug fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.3 &bull;</b> 2021-06-03</p></Typography>
          <Typography use="body1">
            Long-term Sandbox Analytics Storage:
            <ul>
              <li>Metrics object storage deployment</li>
              <li>Thanos integration for metrics data long-term storage</li>
              <li>Maintenance fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.2 &bull;</b> 2021-05-11</p></Typography>
          <Typography use="body1">
            EdgeNative - Connector VM Upgrade:
            <ul>
              <li>CPU/Memory upgrade</li>
              <li>Maintenance fixes</li>
            </ul>
          </Typography>

          <Typography theme="primary" use="subtitle1"><p><b>v1.1 &bull;</b> 2021-03-23</p></Typography>
          <Typography use="body1">
            Monitoring & Analytics:
            <ul>
              <li>Session statistics</li>
              <li>MEC Service & Platform API metrics</li>
              <li>Alerting framework</li>
              <li>Maintenance fixes</li>
            </ul>
          </Typography>

        </div>

      </BasicDialog>
    );
  }
}

const styles = {
  text: {
    // color: 'black',
    marginLeft: 10,
    marginRight: 15
  },
  signature: {
    textAlign: 'right'
  }
};

export default VersionDialog;
