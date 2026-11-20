/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Typography } from '@rmwc/typography';
import { Menu, MenuItem, MenuSurfaceAnchor } from '@rmwc/menu';
import { Button, ButtonIcon } from '@rmwc/button';
import { IconButton } from '@rmwc/icon-button';
import { Elevation } from '@rmwc/elevation';
import { Grid, GridCell } from '@rmwc/grid';
import {
  TopAppBar,
  TopAppBarRow,
  TopAppBarSection
} from '@rmwc/top-app-bar';
import {
  uiChangeCurrentPage,
  uiChangeCurrentDialog,
  uiChangeUserMenuDisplay,
  uiChangeDashboardMenuDisplay,
  uiChangeHelpMenuDisplay,
  uiChangeCurrentDashboard
} from '../state/ui';
import {
  EDGENATIVE_CONNECTOR_VERSION,
  PAGE_HOME,
  PAGE_SANDBOX,
  PAGE_DASHBOARD,
  STATUS_SIGNED_IN,
  DIALOG_HELP_GETTING_STARTED,
  DIALOG_VERSION,
  WIKI_HELP_URL,
  EDGENATIVE_CONNECTOR_WEBINAR_URL,
  WIKI_DISCUSSION_BOARD_URL,
  WIKI_REPORT_ISSUE_URL,
  ETSI_FORGE_URL,
  DASHBOARD_EDGENATIVE_CONNECTOR_URL,
  DASHBOARD_PLATFORM_ADVANTEDGE_URL,
  DASHBOARD_PLATFORM_K8S_URL,
  DASHBOARD_RUNTIME_ENVIRONMENT_NODE_URL,
  DASHBOARD_RUNTIME_ENVIRONMENT_K8S_URL
} from '../app-constants';

/*eslint-disable */
const logo = require('../../img/logo-edgenative-connector.png');
/* eslint-enable */

class TopBarContainer extends Component {
  constructor(props) {
    super(props);

    this.props.changeUserMenuDisplay(false);
    this.props.changeDashboardMenuDisplay(false);
    this.props.changeHelpMenuDisplay(false);
  }

  handleItemClick(page) {
    this.props.changeCurrentPage(page);
  }

  handleItemClickIfEnabled(page, enabled) {
    if (enabled) {
      this.props.changeCurrentPage(page);
    }
  }

  toggleDashboardMenuDisplay() {
    this.props.changeDashboardMenuDisplay(!this.props.dashboardMenuDisplay);
  }

  toggleHelpMenuDisplay() {
    this.props.changeHelpMenuDisplay(!this.props.helpMenuDisplay);
  }

  updateHelpOnSignIn() {
    this.props.changeHelpOnSignIn(this.props.helpOnSignIn);
  }

  render() {
    let isPageHome = (this.props.page === PAGE_HOME);
    let isPageSbox = (this.props.page === PAGE_SANDBOX);
    let isPageDashboard = (this.props.page === PAGE_DASHBOARD);
    let signinButton = (this.props.signInStatus === STATUS_SIGNED_IN) ? 'SIGN OUT' : 'SIGN IN';
    let usernameText = (this.props.signInStatus === STATUS_SIGNED_IN) ? this.props.signInUsername : '';
    let signedIn = (this.props.signInStatus === STATUS_SIGNED_IN);
    let isAdmin = (this.props.signInUserRole === 'admin');

    return (
      <TopAppBar fixed dense style={{ zIndex: 9000 }}>
        <Elevation z={2}>
          <TopAppBarRow theme="primary-bg" style={{height: 32}}/>
          <TopAppBarRow>
            <TopAppBarSection>
              <Grid style={{width: '100%'}}>
                <GridCell span={2}/>
                <GridCell align={'middle'} span={2}>
                  <img
                    id='edgenative-x-logo'
                    className='mdc-top-app-bar__navigation-icon'
                    src={logo}
                    alt=''
                    onClick={() => {
                      window.open(ETSI_FORGE_URL, '_blank');
                    }}
                  />
                </GridCell>

                <GridCell align={'middle'} span={5}>
                  <div align={'right'}>
                    <Button
                      style={isPageHome ? styles.listBtnSelected : styles.listBtn}
                      onClick={() => {this.handleItemClick(PAGE_HOME);}}
                    >
                      Home
                    </Button>
                    <Button
                      disabled={!signedIn}
                      style={signedIn ? (isPageSbox ? styles.listBtnSelected : styles.listBtn) : styles.listBtnDisabled}
                      onClick={() => {this.handleItemClickIfEnabled(PAGE_SANDBOX, signedIn);}}
                    >
                      Sandbox
                    </Button>

                    <MenuSurfaceAnchor style={{ display: (signedIn && isAdmin) ? 'inline-block' : 'none'}}>
                      <Menu
                        open={this.props.dashboardMenuDisplay}
                        onSelect={() => {}}
                        onClose={() => this.props.changeDashboardMenuDisplay(false)}
                        anchorCorner={'bottomLeft'}
                        align={'left'}
                        style={{ whiteSpace: 'nowrap', marginTop: 5 }}
                      >
                        <MenuItem
                          onClick={() => {
                            this.props.changeCurrentDashboard(DASHBOARD_EDGENATIVE_CONNECTOR_URL);
                            this.handleItemClickIfEnabled(PAGE_DASHBOARD, signedIn && isAdmin);
                          }}
                        >
                          <Typography use="body1">EdgeNative - Connector</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            this.props.changeCurrentDashboard(DASHBOARD_PLATFORM_ADVANTEDGE_URL);
                            this.handleItemClickIfEnabled(PAGE_DASHBOARD, signedIn && isAdmin);
                          }}
                        >
                          <Typography use="body1">Platform (Services)</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            this.props.changeCurrentDashboard(DASHBOARD_PLATFORM_K8S_URL);
                            this.handleItemClickIfEnabled(PAGE_DASHBOARD, signedIn && isAdmin);
                          }}
                        >
                          <Typography use="body1">Platform (Kubernetes)</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            this.props.changeCurrentDashboard(DASHBOARD_RUNTIME_ENVIRONMENT_NODE_URL);
                            this.handleItemClickIfEnabled(PAGE_DASHBOARD, signedIn && isAdmin);
                          }}
                        >
                          <Typography use="body1">Runtime Environment (Node)</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            this.props.changeCurrentDashboard(DASHBOARD_RUNTIME_ENVIRONMENT_K8S_URL);
                            this.handleItemClickIfEnabled(PAGE_DASHBOARD, signedIn && isAdmin);
                          }}
                        >
                          <Typography use="body1">Runtime Environment (Kubernetes)</Typography>
                        </MenuItem>
                      </Menu>

                      <Button
                        disabled={!signedIn || !isAdmin}
                        style={signedIn ? (isPageDashboard ? styles.listBtnSelected : styles.listBtn) : styles.listBtnDisabled}
                        onClick={() => this.props.changeDashboardMenuDisplay(true)}
                      >
                        <ButtonIcon icon="arrow_drop_down" />
                        Dashboard
                      </Button>
                    </MenuSurfaceAnchor>

                    <MenuSurfaceAnchor style={{ display: 'inline-block'}}>
                      <Menu
                        open={this.props.userMenuDisplay}
                        onSelect={() => {}}
                        onClose={() => this.props.changeUserMenuDisplay(false)}
                        anchorCorner={'bottomLeft'}
                        align={'left'}
                        style={{ whiteSpace: 'nowrap', marginTop: 5 }}
                      >
                        <MenuItem
                          onClick={() => {
                            this.props.changeCurrentDialog(DIALOG_HELP_GETTING_STARTED);
                          }}
                        >
                          <Typography use="body1">Getting started</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            window.open(EDGENATIVE_CONNECTOR_WEBINAR_URL,'_blank');
                          }}
                        >
                          <Typography use="body1">Webinar</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            window.open(WIKI_HELP_URL,'_blank');
                          }}
                        >
                          <Typography use="body1">Wiki</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            window.open(WIKI_DISCUSSION_BOARD_URL,'_blank');
                          }}
                        >
                          <Typography use="body1">Join Slack</Typography>
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            window.open(WIKI_REPORT_ISSUE_URL,'_blank');
                          }}
                        >
                          <Typography use="body1">Report Issue</Typography>
                        </MenuItem>
                      </Menu>

                      <Button
                        style={styles.btnHelp}
                        onClick={() => this.props.changeUserMenuDisplay(true)}
                      >
                        <ButtonIcon icon="arrow_drop_down" />
                        Help
                      </Button>
                    </MenuSurfaceAnchor>

                    <Button
                      style={styles.btnVersion}
                      raised
                      onClick={() => {
                        this.props.changeCurrentDialog(DIALOG_VERSION);
                      }}
                    >
                      {EDGENATIVE_CONNECTOR_VERSION}
                    </Button>

                  </div>
                </GridCell>

                <GridCell align={'middle'} span={2}>
                  {signedIn ?
                    <MenuSurfaceAnchor style={{ height: 48 }}>
                      <Menu
                        open={this.props.helpMenuDisplay}
                        onSelect={() => {}}
                        onClose={() => this.props.changeHelpMenuDisplay(false)}
                        anchorCorner={'bottomLeft'}
                        align={'left'}
                        style={{ whiteSpace: 'nowrap', marginTop: 5 }}
                      >
                        <MenuItem>
                          <Typography use="body1">Signed in as <b>{usernameText}</b></Typography>
                        </MenuItem>
                        <div style={{ width: '100%', borderTop: '1px solid #e4e4e4'}} />
                        <MenuItem onClick={() => {
                          this.props.onClickSignIn();
                          this.props.changeHelpMenuDisplay(false);
                        }}>
                          <Typography use="body1">Sign out</Typography>
                        </MenuItem>
                      </Menu>

                      <IconButton
                        icon="account_circle"
                        iconOptions={{ size: 'large', id: 'user-icon' }}
                        style={styles.icon}
                        onClick={() => this.props.changeHelpMenuDisplay(true)}
                      />
                    </MenuSurfaceAnchor>
                    :
                    <Button
                      style={styles.btnSignin}
                      onClick={() => this.props.onClickSignIn()}
                    >
                      {signinButton}
                    </Button>
                  }
                </GridCell>
              </Grid>
            </TopAppBarSection>
          </TopAppBarRow>
        </Elevation>
      </TopAppBar>
    );
  }
}

const styles = {
  header: {
    backgroundColor: 'white'
  },
  section: {
    paddingLeft: 15,
    paddingRight: 15,
    paddingTop: 2,
    paddingBottom: 2
  },
  btnSignin: {
    marginTop: 6,
    marginBottom: 6,
    color: '#02488d',
    border: 'none',
    borderRadius: 0
  },
  btnHelp: {
    marginTop: 6,
    marginBottom: 6,
    marginLeft: 10,
    color: '#02488d',
    border: 'none',
    borderRadius: 0
  },
  btnVersion: {
    margin: 6,
    marginLeft: 16,
    // color: 'white',
    fontWeight: 'bold',
    borderRadius: 20
  },
  listBtn: {
    margin: 6,
    color: '#02488d',
    border: 'none',
    borderRadius: 0
  },
  listBtnSelected: {
    margin: 6,
    paddingTop: 3,
    border: 'none',
    borderRadius: 0,
    borderBottom: '3px solid #02488d'
  },
  listBtnDisabled: {
    margin: 6,
    color: 'rgba(2, 72, 141, 0.38)',
    // color: 'rgba(62, 72, 79, 0.38)',
    border: 'none'
  },
  tooltip: {
    arrow: {
      color: 'black'
    }
  },
  icon: {
    color: '#02488d',
    padding: 6
  }
};

const mapStateToProps = state => {
  return {
    page: state.ui.page,
    signInStatus: state.ui.signInStatus,
    signInUsername: state.ui.signInUsername,
    signInUserRole: state.ui.signInUserRole,
    userMenuDisplay: state.ui.userMenuDisplay,
    dashboardMenuDisplay: state.ui.dashboardMenuDisplay,
    helpMenuDisplay: state.ui.helpMenuDisplay,
    helpOnSignIn: state.ui.helpOnSignIn
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeCurrentPage: page => dispatch(uiChangeCurrentPage(page)),
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type)),
    changeUserMenuDisplay: val => dispatch(uiChangeUserMenuDisplay(val)),
    changeDashboardMenuDisplay: val => dispatch(uiChangeDashboardMenuDisplay(val)),
    changeHelpMenuDisplay: val => dispatch(uiChangeHelpMenuDisplay(val)),
    changeCurrentDashboard: url => dispatch(uiChangeCurrentDashboard(url))
  };
};

const ConnectedTopBarContainer = connect(
  mapStateToProps,
  mapDispatchToProps
)(TopBarContainer);

export default ConnectedTopBarContainer;
