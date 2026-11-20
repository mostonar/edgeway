/*
 * Copyright (c) 2022-2025 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Table from '@material-ui/core/Table';
import TableBody from '@material-ui/core/TableBody';
import TableCell from '@material-ui/core/TableCell';
import TableHead from '@material-ui/core/TableHead';
import TableRow from '@material-ui/core/TableRow';
import Paper from '@material-ui/core/Paper';
import { withStyles } from '@material-ui/core/styles';
import { Grid, GridCell } from '@rmwc/grid';


import BasicDialog from './basic-dialog';
import { mapService } from '../../util/string-manipulation';
import {
  DLG_DETAILED_DATA
} from '../../app-constants';

const detailedTableColumnData = [
  [
    { id: 'id', numeric: false, disablePadding: false, label: 'ID' },
    { id: 'service', numeric: false, disablePadding: false, label: 'SERVICE' },
    { id: 'respCode', numeric: false, disablePadding: false, label: 'RESPONSE CODE' },
    { id: 'direction', numeric: false, disablePadding: false, label: 'TYPE' }
  ],
  [
    { id: 'method', numeric: false, disablePadding: false, label: 'METHOD' },
    { id: 'url', numeric: false, disablePadding: false, label: 'ENDPOINT' },
    { id: 'time', numeric: false, disablePadding: false, label: 'TIME' }
  ],
  [
    { id: 'body', numeric: false, disablePadding: false, label: 'REQUEST BODY' }
  ],
  [
    { id: 'respBody', numeric: false, disablePadding: false, label: 'RESPONSE BODY' }
  ]

];

const tableStyles = () => ({
  root: {
    whiteSpace: 'normal'
  },
  table: {
    maxHeight: 360
  },
  tableBody: {
    maxHeight:360
  },
  tableWrapper: {
    maxHeight: 500,
    overflowX: 'auto',
    overflowY: 'scroll'
  },
  tableHeadCell: {
    color: '#FFFFFF',
    'background-color': '#02488d',
    'font-family': 'Rubik',
    'font-size': '0.8rem'
  },
  tableBodyCell: {
    'font-family': 'Rubik'
  }
});

class DetailedDataDialog extends Component {
  constructor(props) {
    super(props);
    this.state = {
      scenarioName: '',
      err: null
    };
    this.classes = props.classes;
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

  render() {

    const classes = this.classes;
    const singleData = this.props.detailedData;

    if (!singleData) {
      return null;
    }

    var serviceId = mapService(singleData.loggerName);
    var id = serviceId + '-' + singleData.id;
    var svcName = 'MEC' + serviceId;
    var reqType = this.mapDirection(singleData.direction);

    var prettyJsonBody = null, prettyJsonRespBody= null;
    var jsonParsed = null;
    if (singleData.body) {
      try {
        jsonParsed = JSON.parse(singleData.body);
        prettyJsonBody = JSON.stringify(jsonParsed, null, 2);
      } catch (e) {
        //send as is
        prettyJsonBody = singleData.body;
      }
    }

    if (singleData.respBody) {
      try {
        jsonParsed = JSON.parse(singleData.respBody);
        prettyJsonRespBody = JSON.stringify(jsonParsed, null, 2);
      } catch (e) {
        //send as is
        prettyJsonRespBody = singleData.respBody;
      }
    }

    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onClose={this.props.onClose}
        onSubmit={this.props.onClose}
        submitLabel = {'CLOSE'}
        cydata={DLG_DETAILED_DATA}
        className={'detailed-dialog'}
      >
        <Grid className={classes.root}>
          <GridCell span={12}>
            <Paper>
              <Table
                className={classes.table}
                aria-labelledby="tableTitle"
              >
                <TableHead>
                  <TableRow>
                    {detailedTableColumnData[0].map(column => {
                      return (
                        <TableCell
                          key={column.time}
                          numeric={column.numeric}
                          padding={column.disablePadding ? 'none' : 'default'}
                          className={classes.tableHeadCell}
                        >
                          {column.label}
                        </TableCell>
                      );
                    }, this)}
                  </TableRow>
                </TableHead>
                <TableBody className={classes.tableBody}>
                  <TableRow>
                    <TableCell className={classes.tableBodyCell} component="th" scope="row">
                      {id}
                    </TableCell>
                    <TableCell className={classes.tableBodyCell}>
                      {svcName}
                    </TableCell>
                    <TableCell className={classes.tableBodyCell}>
                      {singleData.respCode}
                    </TableCell>
                    <TableCell className={classes.tableBodyCell}>
                      {reqType}
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </Paper>
          </GridCell>

          <GridCell span={12}>
            <Paper>
              <Table
                className={classes.table}
                aria-labelledby="tableTitle"
              >
                <TableHead>
                  <TableRow>
                    {detailedTableColumnData[1].map(column => {
                      return (
                        <TableCell
                          key={column.time}
                          numeric={column.numeric}
                          padding={column.disablePadding ? 'none' : 'default'}
                          className={classes.tableHeadCell}
                        >
                          {column.label}
                        </TableCell>
                      );
                    }, this)}
                  </TableRow>
                </TableHead>
                <TableBody className={classes.tableBody}>
                  <TableRow>
                    <TableCell className={classes.tableBodyCell} component="th" scope="row">
                      {singleData.method}
                    </TableCell>
                    <TableCell className={classes.tableBodyCell}>
                      {singleData.url}
                    </TableCell>
                    <TableCell className={classes.tableBodyCell}>
                      {singleData.time}
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </Paper>
          </GridCell>

          <GridCell span={12}>
            <Paper>
              <Table
                className={classes.table}
                aria-labelledby="tableTitle"
                // style={{ width: '100%' }}
              >
                <TableHead>
                  <TableRow>
                    {detailedTableColumnData[2].map(column => {
                      return (
                        <TableCell
                          key={column.time}
                          numeric={column.numeric}
                          padding={column.disablePadding ? 'none' : 'default'}
                          className={classes.tableHeadCell}
                        >
                          {column.label}
                        </TableCell>
                      );
                    }, this)}
                  </TableRow>
                </TableHead>
                <TableBody className={classes.tableBody}>
                  <TableRow>
                    <TableCell component="th" scope="row">
                      <pre>{prettyJsonBody}</pre>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </Paper>
          </GridCell>

          <GridCell span={12}>
            <Paper>
              <Table
                className={classes.table}
                aria-labelledby="tableTitle"
              >
                <TableHead>
                  <TableRow>
                    {detailedTableColumnData[3].map(column => {
                      return (
                        <TableCell
                          key={column.time}
                          numeric={column.numeric}
                          padding={column.disablePadding ? 'none' : 'default'}
                          className={classes.tableHeadCell}
                        >
                          {column.label}
                        </TableCell>
                      );
                    }, this)}
                  </TableRow>
                </TableHead>
                <TableBody className={classes.tableBody}>
                  <TableRow>
                    <TableCell component="th" scope="row">
                      <pre>{prettyJsonRespBody}</pre>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </Paper>
          </GridCell>
        </Grid>
      </BasicDialog>
    );
  }
}

DetailedDataDialog.propTypes = {
  classes: PropTypes.object.isRequired
};

export default withStyles(tableStyles)(DetailedDataDialog);
