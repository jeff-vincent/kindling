// config.js — centralised settings.
// Nothing is validated; services log warnings and degrade gracefully.

module.exports = {
  SERVER_PORT: 3000,
  MONGO_URL: process.env.MONGO_URL || "",
  // Redis used exclusively for consuming the order_events queue
  // (shared with orders service).
  EVENT_STORE_URL: process.env.EVENT_STORE_URL || "",
};
