/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component } from 'react';
import Iframe from 'react-iframe';

class DashboardPageContainer extends Component {
  constructor(props) {
    super(props);
  }

  render() {
    return (
      <div className='ui-dashboard'>
        <Iframe
          url={this.props.currentDashboard}
          id="myId"
          display="initial"
          position="relative"
          allowFullScreen
          width='100%'
          height='100%'
          frameBorder ="0"
        />
      </div>
    );
  }
}

const mapStateToProps = state => {
  return {
    page: state.ui.page,
    currentDashboard: state.ui.currentDashboard
  };
};

const mapDispatchToProps = () => {
  return {
  };
};

const ConnectedDashboardPageContainer = connect(
  mapStateToProps,
  mapDispatchToProps
)(DashboardPageContainer);

export default ConnectedDashboardPageContainer;
