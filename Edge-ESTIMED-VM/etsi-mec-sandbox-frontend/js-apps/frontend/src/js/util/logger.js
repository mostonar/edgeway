import log from 'loglevel';
import { LOG_TYPES } from './log-types';

const COLORS = {
  [LOG_TYPES.TRACE]: 'magenta',
  [LOG_TYPES.DEBUG]: 'cyan',
  [LOG_TYPES.INFO]: 'blue',
  [LOG_TYPES.WARN]: 'goldenrod',
  [LOG_TYPES.ERROR]: 'red'
};

const originalFactory = log.methodFactory;
log.methodFactory = function (methodName, logLevel, loggerName) {
  const rawMethod = originalFactory(methodName, logLevel, loggerName);
  return function (...args) {
    const level = methodName.toUpperCase();
    const color = COLORS[level] || 'black';
    const timestamp = new Date().toTimeString().replace(/.*(\d{2}:\d{2}:\d{2}).*/, '$1');
    const prefix = `[${timestamp}] ${level}${loggerName ? ` (${loggerName})` : ''}:`;

    // Use %c to format the prefix with CSS matching meepctl colors
    rawMethod(`%c${prefix}`, `color: ${color}; font-weight: bold;`, ...args);
  };
};

// Set default level based on environment variable

const envLevel = process.env.LOG_LEVEL;
if (envLevel) {
  log.setLevel(envLevel);
} else if (process.env.NODE_ENV === 'production') {
  log.setLevel(LOG_TYPES.INFO);
} else {
  log.setLevel(LOG_TYPES.DEBUG);
}


export default log;
