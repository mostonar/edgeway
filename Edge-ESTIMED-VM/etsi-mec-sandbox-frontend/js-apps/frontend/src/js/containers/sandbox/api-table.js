/*
 * Copyright (c) 2022-2025 ETSI.  All rights reserved.
 */

import PropTypes from 'prop-types';
import Table from '@material-ui/core/Table';
import TableBody from '@material-ui/core/TableBody';
import TableCell from '@material-ui/core/TableCell';
import TableHead from '@material-ui/core/TableHead';
import TableRow from '@material-ui/core/TableRow';
import Paper from '@material-ui/core/Paper';
import { connect } from 'react-redux';
import React, { Component } from 'react';
import { withStyles } from '@material-ui/core/styles';
import { getSorting } from '../../util/table-utils';
import { TextField } from '@rmwc/textfield';
import { Select } from '@rmwc/select';
import { Checkbox } from '@rmwc/checkbox';
import { Button } from '@rmwc/button';
import { sboxChangeApiTable } from '../../state/sbox';
import { mapService } from '../../util/string-manipulation';
import {
  uiSandboxChangeApiDetailedData,
  uiChangeCurrentDialog
} from '../../state/ui';
import {
  DIALOG_DETAILED_DATA,
  DEFAULT_NO_NETWORK_FILE_SELECTED,
  MEC_SERVICES
} from '../../app-constants';

const tableStyles = () => ({
  root: {
    width: '100%'
  },
  table: {
    maxHeight: 650
  },
  tableBody: {
    maxHeight: 650
  },
  tableWrapper: {
    maxHeight: 650,
    overflowX: 'auto',
    overflowY: 'scroll'
  },
  tableHeadCell: {
    paddingLeft: 15,
    paddingRight: 0,
    color: '#FFFFFF',
    'background-color': '#02488d',
    'font-family': 'Rubik',
    'font-size': '0.8rem',
    'font-weight': 500
  },
  tableBodyCell: {
    paddingLeft: 15,
    paddingRight: 0,
    'font-family': 'Rubik'
  },
  tableBodyCellNoWrap: {
    paddingLeft: 15,
    paddingRight: 0,
    'font-family': 'Rubik',
    whiteSpace: 'nowrap'
  },
  toolbarContainer: {
    display: 'flex',
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    flexWrap: 'wrap',
    padding: '8px 15px',
    borderBottom: '1px solid #d9e2ec',
    backgroundColor: '#ecf3f9',
    gap: '12px'
  },
  filterGroup: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    flexWrap: 'wrap',
    gap: '12px'
  },
  actionGroup: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    gap: '12px'
  },
  filterField: {
    minWidth: '160px',
    width: '160px'
  },
  searchField: {
    minWidth: '180px',
    width: '180px'
  },
  checkboxLabel: {
    display: 'flex',
    alignItems: 'center',
    fontFamily: 'Rubik',
    fontSize: '0.9rem',
    marginLeft: '5px'
  },
  button: {
    textTransform: 'none',
    fontFamily: 'Rubik'
  },
  clearButton: {
    textTransform: 'none',
    fontFamily: 'Rubik',
    color: '#d32f2f',
    borderColor: '#d32f2f'
  }
});

const serviceOptions = [
  { label: 'All Services', value: 'ALL' },
  ...MEC_SERVICES.map(svc => ({
    label: `${svc.code} - ${svc.name}`,
    value: svc.code
  }))
];

const methodOptions = [
  { label: 'All Methods', value: 'ALL' },
  { label: 'GET', value: 'GET' },
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'DELETE', value: 'DELETE' },
  { label: 'PATCH', value: 'PATCH' }
];

const tableColumnData = [
  {
    id: 'id',
    numeric: false,
    disablePadding: false,
    label: 'ID'
  },
  {
    id: 'respCode',
    numeric: false,
    disablePadding: true,
    label: 'RESP. CODE'
  },
  {
    id: 'direction',
    numeric: false,
    disablePadding: true,
    label: 'TYPE'
  },
  {
    id: 'method',
    numeric: false,
    disablePadding: true,
    label: 'METHOD'
  },
  {
    id: 'endpoint',
    numeric: false,
    disablePadding: false,
    label: 'ENDPOINT'
  },
  {
    id: 'time',
    numeric: false,
    disablePadding: true,
    label: 'TIME'
  }
];

class ApiTable extends Component {
  constructor(props) {
    super(props);
    this.classes = props.classes;
    this.state = {
      searchTerm: '',
      serviceFilter: 'ALL',
      methodFilter: 'ALL',
      errorsOnly: false
    };
  }

  exportLogs(filteredData) {
    if (!filteredData || !filteredData.length) {
      return;
    }
    const jsonString = `data:text/json;charset=utf-8,${encodeURIComponent(
      JSON.stringify(filteredData, null, 2)
    )}`;
    const downloadAnchor = document.createElement('a');
    downloadAnchor.setAttribute('href', jsonString);
    downloadAnchor.setAttribute('download', `api_logs_${new Date().toISOString()}.json`);
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
  }

  onClick(event, row) {
    this.props.changeDetailedData(row);
    this.props.changeCurrentDialog(DIALOG_DETAILED_DATA);
  }



  mapDirection(direction) {
    switch(direction) {
    case 'RX':
      return 'Request';
    case 'TX':
      return 'Notification';
    default:
      return 'N/A';
    }
  }

  formatTime(timeStr) {
    if (!timeStr) {
      return '';
    }
    try {
      const d = new Date(timeStr);
      if (isNaN(d.getTime())) {
        return timeStr;
      }
      const year = d.getFullYear();
      const month = String(d.getMonth() + 1).padStart(2, '0');
      const day = String(d.getDate()).padStart(2, '0');
      const hours = String(d.getHours()).padStart(2, '0');
      const minutes = String(d.getMinutes()).padStart(2, '0');
      const seconds = String(d.getSeconds()).padStart(2, '0');
      return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
    } catch (e) {
      return timeStr;
    }
  }

  render() {
    const classes = this.classes;
    var data = null;
    if (this.props.apiTable && this.props.networkFileSelected !== DEFAULT_NO_NETWORK_FILE_SELECTED) {
      data = this.props.apiTable.values;
    }
    // const data = this.props.table.entries;
    const order = 'desc';
    const orderBy = 'time';

    // Apply filters client-side
    let filteredData = data || [];
    if (this.props.networkFileSelected === DEFAULT_NO_NETWORK_FILE_SELECTED) {
      filteredData = [];
    }

    if (filteredData.length > 0) {
      const { searchTerm, serviceFilter, methodFilter, errorsOnly } = this.state;

      filteredData = filteredData.filter(row => {
        // 1. Search term filter (endpoint)
        if (searchTerm) {
          const endpointLower = (row.endpoint || '').toLowerCase();
          if (!endpointLower.includes(searchTerm.toLowerCase())) {
            return false;
          }
        }

        // 2. Service Filter
        if (serviceFilter !== 'ALL') {
          const serviceMapped = mapService(row.loggerName);
          if (!serviceMapped.startsWith(serviceFilter)) {
            return false;
          }
        }

        // 3. Method Filter
        if (methodFilter !== 'ALL') {
          if ((row.method || '') !== methodFilter) {
            return false;
          }
        }

        // 4. Errors Only
        if (errorsOnly) {
          const code = parseInt(row.respCode, 10);
          if (isNaN(code) || code < 400) {
            return false;
          }
        }

        return true;
      });
    }

    const maxRows = 12;
    var emptyRows = 0;
    if (!filteredData || !filteredData.length) {
      emptyRows = maxRows;
    } else {
      emptyRows = Math.max(0, maxRows - filteredData.length);
    }

    return (
      <Paper className={classes.root}>
        {/* API Console Toolbar */}
        <div className={classes.toolbarContainer}>
          <div className={classes.filterGroup}>
            <TextField
              outlined
              label="Search endpoint..."
              value={this.state.searchTerm}
              onChange={e => this.setState({ searchTerm: e.target.value })}
              className={classes.searchField}
            />
            <Select
              outlined
              label="Service"
              value={this.state.serviceFilter}
              options={serviceOptions}
              onChange={e => this.setState({ serviceFilter: e.target.value })}
              className={classes.filterField}
            />
            <Select
              outlined
              label="Method"
              value={this.state.methodFilter}
              options={methodOptions}
              onChange={e => this.setState({ methodFilter: e.target.value })}
              className={classes.filterField}
            />
            <Checkbox
              checked={this.state.errorsOnly}
              onChange={e => this.setState({ errorsOnly: e.target.checked })}
              className={classes.checkboxLabel}
            >
              Errors Only
            </Checkbox>
          </div>
          <div className={classes.actionGroup}>
            <Button
              outlined
              className={classes.button}
              onClick={() => this.exportLogs(filteredData)}
              disabled={!filteredData.length}
            >
              Export
            </Button>
            <Button
              outlined
              className={classes.clearButton}
              onClick={() => this.props.clearApiTable()}
              disabled={!data || !data.length}
            >
              Clear
            </Button>
          </div>
        </div>

        <div className={classes.tableWrapper}>
          <Table
            className={classes.table}
            aria-labelledby="tableTitle"
            style={{ width: '100%' }}
          >
            <TableHead>
              <TableRow title="View MEC API requests & notifications">
                {tableColumnData.map(column => {
                  return (
                    <TableCell
                      key={column.id}
                      numeric={column.numeric}
                      padding={column.disablePadding ? 'none' : 'default'}
                      sortDirection={
                        orderBy === column.id ? order : false
                      }
                      className={classes.tableHeadCell}
                    >
                      {column.label}
                    </TableCell>
                  );
                }, this)}
              </TableRow>
            </TableHead>
            {emptyRows < maxRows && <TableBody className={classes.tableBody} title="Click to get detailed info">
              {filteredData
                .sort(getSorting(order, orderBy))
                .map((row) => {
                  return (
                    <TableRow
                      hover
                      onClick={event => this.onClick(event, row)}
                      role="checkbox"
                      tabIndex={-1}
                      key={row.time}
                    >
                      <TableCell className={classes.tableBodyCellNoWrap} scope="row">
                        {mapService(row.loggerName) + '-' + row.id}
                      </TableCell>
                      <TableCell className={classes.tableBodyCell} padding='none'>
                        {row.respCode}
                      </TableCell>
                      <TableCell className={classes.tableBodyCell} padding='none'>
                        {this.mapDirection(row.direction)}
                      </TableCell>
                      <TableCell className={classes.tableBodyCell} padding='none'>
                        {row.method}
                      </TableCell>
                      <TableCell className={classes.tableBodyCell}>
                        {row.endpoint}
                      </TableCell>
                      <TableCell className={classes.tableBodyCell} padding='none'>
                        {this.formatTime(row.time)}
                      </TableCell>
                    </TableRow>
                  );
                })
              }
              {emptyRows > 0 && (
                <TableRow style={{ height: 49 * emptyRows }}>
                  <TableCell colSpan={6} />
                </TableRow>
              )}
            </TableBody>
            }
            {emptyRows === maxRows && (
              <TableBody className={classes.tableBody}>
                <TableRow style={{ height: 49 * emptyRows }}>
                  <TableCell colSpan={6} />
                </TableRow>
              </TableBody>
            )}
          </Table>
        </div>
      </Paper>
    );
  }
}

ApiTable.propTypes = {
  classes: PropTypes.object.isRequired
};

const mapStateToProps = state => {
  return {
    order: state.ui.order,
    apiTable: state.sbox.apiTable.data,
    networkFileSelected: state.ui.networkFileSelected
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeDetailedData: row => dispatch(uiSandboxChangeApiDetailedData(row)),
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type)),
    clearApiTable: () => dispatch(sboxChangeApiTable({ values: [] }))
  };
};

export default withStyles(tableStyles)(
  connect(
    mapStateToProps,
    mapDispatchToProps
  )(ApiTable)
);
