import axios from "axios";
import { getBackendBaseURL } from "../config/runtime";

export const api = axios.create({
  baseURL: getBackendBaseURL(),
  timeout: 15000
});

export function syncApiBaseURL() {
  api.defaults.baseURL = getBackendBaseURL();
}
