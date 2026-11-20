import { faro } from '@grafana/faro-web-sdk';

export const telemetryMiddleware = store => next => action => {
  const result = next(action);

  if (faro && faro.api) {
    faro.api.pushEvent('redux_action', {
      action_type: action.type,
      payload: action.payload !== undefined ? JSON.stringify(action.payload) : 'undefined'
    });
  }

  return result;
};
