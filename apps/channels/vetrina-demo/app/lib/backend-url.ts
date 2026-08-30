// Where server-side code reaches the backend, decided ONCE - this lived in six modules, and when
// the default moved from the compose hostname to the host port five of them were missed
//
// The default is the host port so `npm run dev` outside Docker works; compose sets the in-network
// app:8080 explicitly for the container path
export const BACKEND_INTERNAL_URL =
  process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080";
