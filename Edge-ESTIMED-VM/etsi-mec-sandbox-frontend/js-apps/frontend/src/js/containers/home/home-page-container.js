/*
 * Copyright (c) 2022-2025 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Grid, GridCell, GridInner } from '@rmwc/grid';
// import { Elevation } from '@rmwc/elevation';
import { Typography } from '@rmwc/typography';

import {
  uiChangeCurrentDialog
} from '../../state/ui';

import {
  ETSI_FORGE_MEC_URL,
  ETSI_FORGE_URL,
  EDGENATIVE_CONNECTOR_OVERVIEW_PDF_URL,
  ETSI_MEC_URL,
  ETSI_PORTAL_URL,
  PAGE_HOME
} from '../../app-constants';


class HomePageContainer extends Component {
  constructor(props) {
    super(props);
  }

  render() {
    if (this.props.page !== PAGE_HOME) {
      return null;
    }

    return (
      <Grid style={styles.headlineGrid}>
        <GridCell style={styles.banner} span={12}>
          <GridInner>
            <GridCell style={{height: 20}} span={12}/>
            <GridCell span={2}/>
            <GridCell span={10}>
              <Typography style={styles.bannerText} use="headline2">EdgeNative - Connector</Typography>
            </GridCell>
            <GridCell span={2}/>
            <GridCell span={10}>
              <Typography style={styles.bannerText} use="headline4">Experience MEC APIs</Typography>
            </GridCell>
          </GridInner>
        </GridCell>

        <GridCell theme="primary-bg" style={{height: 32, marginTop: -10}} span={12}/>

        <GridCell span={2}/>
        <GridCell span={8}>
          <div style={styles.title}>
            <Typography theme="primary" use="headline4">Welcome to EdgeNative - Connector portal</Typography>
          </div>
          <div>
            <Typography className="text-color-main" use="body1">
              <p>
                EdgeNative - Connector is an interactive environment that enables users to learn & experiment with ETSI MEC Service
                APIs. These standardised RESTful APIs are targeted towards MEC application developers to expose the value
                added services offered by MEC, including real time access to network and context information, as well as
                location awareness. The design principles for developing the APIs have also been specified in ETSI GS MEC 009,
                along with http methods, templates, conventions and patterns. The MEC service APIs are available in YAML
                and JSON format at <a href={ETSI_FORGE_URL} target="_blank">{ETSI_FORGE_URL}</a>,
                presented via OpenAPI compliant descriptions.
              </p>
              <p>
                EdgeNative - Connector provides the user with a choice of scenarios combining different network technologies
                (4G, 5G, Wi-Fi, MEC Federation) and terminal types. Combining these assets in a geolocated environment, a user can gain
                hands-on experience on the behaviour and capabilities of the Location (MEC013), Radio Network
                Information (MEC012), Traffic Management APIs (MEC015), Device application interface (MEC016), Application Mobility (MEC021), WLAN Information (MEC028),
                V2X Information API (MEC030) service API, MEC IoT API (MEC 033), MEC Federation Service API (MEC040) service APIs and Sensors-Sharing Services (MEC046).
                EdgeNative - Connector provides also a support for Edge Native Connector (3GPP TS 29.222: 3rd Generation Partnership Project; Technical Specification Group Core
                Network and Terminals; Common API Framework for 3GPP Northbound APIs).
              </p>
              <p>
                Such contextual information can offer significant differential performance for edge based MEC applications.
                EdgeNative - Connector also provides the support of capabilities described by Edge Platform Application Enablement (MEC011) and Device Application Interface (MEC016).
              </p>
              <p>
                <b>Important Note: If MEC Federation Service API (MEC040) is used (e.g. by activation 4g-5g-wifi-macro-v2x-fed network scenarion), MEC services and
                  MEC applications will be visible from other MEC Platform using also MEC Federation Service API (MEC040).
                  If you choose to use MEC Federation Service API (MEC040), you accept to establish MEC federation between two partners as specified in ETSI GS MEC 040 V3.2.1 (2024-03) Clause 5.2.4.1 Introduction.
                </b>
              </p>
              <p>
                Users can use the EdgeNative - Connector API to develop their own MEC applications to access and use the live MEC APIs running an instance of EdgeNative - Connector.
                The demo6 example illustrates how to use the EdgeNative - Connector API.
                The demo9 example illustrates how to use the EdgeNative - Connector API jointly with oneM2M platform.
              </p>
              <p>
                But wait - EdgeNative - Connector is not only a browser experience!<br/>
                Users with existing applications can configure them to access and use the live MEC APIs running in their own
                test environment.
              </p>
              <p>
                Interested to learn more?<br/>
                Follow the help menu links (Webinar, Wiki, Community, etc.) to get started or simply sign-in and try-it!
              </p>
              <p>
                The following resources provide more information on ETSI MEC:<br/>
                - <a href={ETSI_MEC_URL} target="_blank">Multi-access Edge Computing</a><br/>
                - <a href={ETSI_PORTAL_URL} target="_blank">ETSI MEC Portal</a><br/>
                - <a href={ETSI_FORGE_MEC_URL} target="_blank">ETSI Forge</a><br/>
                - <a href={EDGENATIVE_CONNECTOR_OVERVIEW_PDF_URL} target="_blank">ETSI MEC Slide Deck</a>
              </p>
            </Typography>
          </div>
        </GridCell>
      </Grid>
    );
  }
}

const styles = {
  headlineGrid: {
    marginBottom: 10
  },
  banner: {
    backgroundSize: 'cover',
    backgroundRepeat: 'no-repeat',
    backgroundPosition: 'center',
    backgroundImage: 'url(../../../img/banner-edgenative-connector.png)',
    width: '100%',
    height: 500
  },
  bannerText: {
    color: '#fff',
    textShadow: '2px 2px 3px #000'
  },
  headline: {
    padding: 10
  },
  paragraph: {
    marginBottom: 10
  },
  title: {
    marginTop: 10,
    marginBottom: 20
  }
};

const mapStateToProps = state => {
  return {
    page: state.ui.page
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type))
  };
};

const ConnectedHomePageContainer = connect(
  mapStateToProps,
  mapDispatchToProps
)(HomePageContainer);

export default ConnectedHomePageContainer;
