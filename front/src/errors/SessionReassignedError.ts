export class SessionReassignedError extends Error {
  constructor() {
    super("Session reassigned");
    this.name = "SessionReassignedError";
  }
}
