/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import PropTypes from 'prop-types';
import Table from '@material-ui/core/Table';
import TableBody from '@material-ui/core/TableBody';
import TableCell from '@material-ui/core/TableCell';
import TableRow from '@material-ui/core/TableRow';
import Paper from '@material-ui/core/Paper';
import Typography from '@material-ui/core/Typography';
import { connect } from 'react-redux';
import React, { Component } from 'react';
import { withStyles } from '@material-ui/core/styles';
import { getSorting } from '../../util/table-utils';
import { Checkbox } from '@rmwc/checkbox';
import {
  uiSandboxChangeApiDetailedData,
  uiChangeCurrentDialog,
  uiSandboxChangeAppInstancesSelected
} from '../../state/ui';

import {
  DEFAULT_NO_NETWORK_FILE_SELECTED
} from '../../app-constants';

const tableStyles = () => ({
  root: {
    width: '100%'
  },
  table: {
    maxHeight: 150
  },
  tableBody: {
    maxHeight: 150
  },
  tableWrapper: {
    maxHeight: 150,
    overflowX: 'auto',
    overflowY: 'scroll'
  },
  tableBodyCell: {
    paddingLeft: 5,
    paddingRight: 0,
    'font-family': 'Rubik'
  },
  tableBodyCellNoWrap: {
    paddingLeft: 10,
    paddingRight: 0,
    'font-family': 'Rubik',
    whiteSpace: 'nowrap'
  }
});

class AppInstanceTable extends Component {
  constructor(props) {
    super(props);
    this.state = {
      dismissibleOpen: true
    };
    this.classes = props.classes;
  }

  updateAppInstancesToDelete(appInstanceId, checked) {
    var appInstancesToDelete = this.props.appInstancesSelected;
    if (checked) {
      appInstancesToDelete.push(appInstanceId);
      this.props.changeAppInstancesSelected(appInstancesToDelete);
    } else {
      appInstancesToDelete = appInstancesToDelete.filter(item => item !== appInstanceId);
      this.props.changeAppInstancesSelected(appInstancesToDelete);
    }
  }

  render() {
    const classes = this.classes;
    var data = (this.props.networkFileSelected !== DEFAULT_NO_NETWORK_FILE_SELECTED) ?
      this.props.appInstanceTable : null;

    const order = 'desc';
    const orderBy = 'name';
    let userApps = [];
    let systemApps = [];

    if (data) {
      let sortedData = data.sort(getSorting(order, orderBy));
      userApps = sortedData.filter(row => row.type === 'USER');
      systemApps = data.filter(row => row.type === 'SYSTEM').sort((a, b) => {
        if (a.nodeName < b.nodeName) {
          return -1;
        }
        if (a.nodeName > b.nodeName) {
          return 1;
        }
        if (a.name < b.name) {
          return -1;
        }
        if (a.name > b.name) {
          return 1;
        }
        return 0;
      });
    }

    const maxRows = 3;
    const totalRenderedRows = userApps.length + systemApps.length + (userApps.length > 0 ? 1 : 0) + (systemApps.length > 0 ? 1 : 0);
    const emptyRows = Math.max(0, maxRows - totalRenderedRows);

    return (
      <Paper className={classes.root}>
        <div className={classes.tableWrapper}>
          <Table
            className={classes.table}
            aria-labelledby="tableTitle"
            style={{ width: '100%' }}
          >
            {data && data.length > 0 && <TableBody className={classes.tableBody}>
              {userApps.length > 0 && (
                <TableRow style={{ backgroundColor: '#f5f5f5', height: 30 }}>
                  <TableCell colSpan={2} padding="none" style={{ paddingLeft: 10 }}>
                    <Typography variant="subtitle2" color="textSecondary">USER APPLICATIONS</Typography>
                  </TableCell>
                </TableRow>
              )}
              {userApps.map(row => (
                <TableRow
                  hover
                  role="checkbox"
                  tabIndex={-1}
                  key={row.id}
                  style={{ height: 49 }}
                >
                  <TableCell className={classes.tableBodyCell} padding="none">
                    <Checkbox
                      onChange={e => {
                        this.updateAppInstancesToDelete(row.id, e.target.checked);
                      }}
                    />
                  </TableCell>
                  <TableCell className={classes.tableBodyCellNoWrap} scope="row">
                    {row.name} running on {row.nodeName}<br/>
                    ID: {row.id}
                  </TableCell>
                </TableRow>
              ))}

              {systemApps.length > 0 && (
                <TableRow style={{ backgroundColor: '#f5f5f5', height: 30 }}>
                  <TableCell colSpan={2} padding="none" style={{ paddingLeft: 10 }}>
                    <Typography variant="subtitle2" color="textSecondary">MEC SERVICES</Typography>
                  </TableCell>
                </TableRow>
              )}
              {systemApps.map(row => (
                <TableRow
                  hover
                  tabIndex={-1}
                  key={row.id}
                  style={{ height: 49 }}
                >
                  <TableCell className={classes.tableBodyCell} padding="none">
                    <Checkbox disabled />
                  </TableCell>
                  <TableCell className={classes.tableBodyCellNoWrap} scope="row">
                    {row.name} running on {row.nodeName}<br/>
                    ID: {row.id}
                  </TableCell>
                </TableRow>
              ))}

              {emptyRows > 0 && (
                <TableRow style={{ height: 49 * emptyRows }}>
                  <TableCell colSpan={6} />
                </TableRow>
              )}
            </TableBody>
            }
            {(!data || data.length === 0) && (
              <TableBody className={classes.tableBody}>
                <TableRow style={{ height: 49 * maxRows }}>
                  <TableCell colSpan={6} style={{ textAlign: 'center', color: '#757575', borderBottom: 'none' }}>
                    <Typography variant="body1">No Applications Running</Typography>
                  </TableCell>
                </TableRow>
              </TableBody>
            )}
          </Table>
        </div>
      </Paper>
    );
  }
}

AppInstanceTable.propTypes = {
  classes: PropTypes.object.isRequired
};

const mapStateToProps = state => {
  return {
    order: state.ui.order,
    appInstanceTable: state.sbox.appInstanceTable.data,
    appInstancesSelected: state.ui.appInstancesSelected,
    networkFileSelected: state.ui.networkFileSelected
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeDetailedData: row => dispatch(uiSandboxChangeApiDetailedData(row)),
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type)),
    changeAppInstancesSelected: list => dispatch(uiSandboxChangeAppInstancesSelected(list))
  };
};

export default withStyles(tableStyles)(
  connect(
    mapStateToProps,
    mapDispatchToProps
  )(AppInstanceTable)
);
