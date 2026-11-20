/* Operating decisions use the phase before '/'; record details keep the full value. */
const RobotState = {
  primary(value) {
    return typeof value === 'string' ? value.split('/', 1)[0].trim() || null : null;
  }
};
if (typeof module !== 'undefined' && module.exports) module.exports = RobotState;
