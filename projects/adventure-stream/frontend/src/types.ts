export interface StreamStatus {
  live: boolean;
  title?: string;
  description?: string;
  started_at?: string;
}

export interface StreamToken {
  token: string;
  url: string;
  room: string;
  identity: string;
}

export interface IngressInfo {
  ingress_id: string;
  url: string;
  stream_key: string;
  room: string;
  participant_identity: string;
  participant_name: string;
  input_type: string;
}

export interface Participant {
  sid: string;
  identity: string;
  name: string;
  metadata: string;
  state: string;
  joined_at: number;
  region: string;
}

export interface RoomInfo {
  sid: string;
  name: string;
  empty_timeout: number;
  max_participants: number;
  metadata: string;
  num_participants: number;
  creation_time: number;
  active_recording: boolean;
}

export interface AdminStreamStatus {
  stream: {
    title: string;
    description: string;
    started_at: string;
    room_sid: string;
  } | null;
  rooms: RoomInfo[];
  ingress: IngressInfo[];
}

export interface LoginResponse {
  token: string;
}
