
const grid = document.getElementById("video-grid");
const localVideo = document.getElementById("localVideo");

const randButton = document.getElementById("randButton");
const startButton = document.getElementById("startButton");
const callButton = document.getElementById("callButton");
const hangupButton = document.getElementById("hangupButton");

let ws;
let localStream;

let rtcPeers = new Map();

const offerOptions = {
  offerToReceiveAudio: 1,
  offerToReceiveVideo: 1
};


if (randButton) {randButton.addEventListener("click", randRoom);}
if (startButton) {
  startButton.disabled = false;
  startButton.addEventListener("click", start);
}
if (callButton) {
  callButton.disabled = true;
  callButton.addEventListener("click", call);
}
if (hangupButton) {
  hangupButton.disabled = true;
  hangupButton.addEventListener("click", hangup);
}

async function start() {
  console.log("Requesting local stream");
  startButton.disabled = true;
  try {
    const stream = await navigator.mediaDevices.getUserMedia({audio: true, video: true});
    console.log("Received local stream");
    localVideo.srcObject = stream;
    localStream = stream;
    callButton.disabled = false;
  } catch (e) {
    alert(`getUserMedia() error: ${e.name}`);
  }
}

async function call() {
  callButton.disabled = true;
  hangupButton.disabled = false;
  let urlParams = new URLSearchParams(window.location.search);
  let roomID = urlParams.get("id");
  let username = urlParams.get("username");
  console.log(roomID);
  ws = new WebSocket("ws://" +location.host+ "/ws?roomid=" +roomID+ "&username=" +username);
  ws.onopen = wsOnOpen;
  ws.onclose = wsOnClose;
  ws.onmessage = wsOnMessage;
  ws.onerror = wsOnError;
}

function wsOnOpen (event) {
  console.log("trying to connect to ws")
  // Send request to connect to peers in room
  // createPeerConnection();
  console.log("sending join");
  ws.send(JSON.stringify({Data: {join: true}}));
};

function wsOnClose (event) {
  if (event.wasClean) {
    console.log("ws conn closed clean.")
  } else {
    console.log("ws conn broken.", event);
  }
  console.log("code: " + event.code + " reason: " + event.reason);
  // close all rtp and remove videoboxes
  cleanUpAll()
  // close local mediastreams
  localVideo.srcObject.getTracks().forEach(track => track.stop());
}

function wsOnMessage (event) {
  let msg = JSON.parse(event.data);
  // Join EVENT
  if(msg.Data.join) {
    console.log("getting mssage join: ", msg.ClientID, msg.Data.join);
    onJoin(msg);
  // Offer received EVENT/ sdp
  } else if (msg.Data.offer){
    console.log("mssage offer: ", msg.ClientID, msg.Data.offer);
    onOffer(msg);
  // Answer received EVENT/ sdp
  } else if (msg.Data.answer) {
    console.log("getting answer: ", msg.ClientID, msg.Data.answer);
    onAnswer(msg);
  // Icecandidate EVENT
  } else if (msg.Data.iceCandidate) {
    console.log("getting mssage icecandidate: ", msg.ClientID, msg.Data.iceCandidate);
    addIceCandidate(msg);
  } else if (msg.Data.hangup) {
    console.log("getting mssage hangup: ", msg.ClientID, msg.Data.hangup);
    cleanUpID(msg.ClientID)
  }
   else {
    console.log("Unknown message type: ", msg);
  }
};

function wsOnError(error) {
    console.log("ws error:", error.message);
};

function createPeerConnection() {
  const videoTracks = localStream.getVideoTracks();
  const audioTracks = localStream.getAudioTracks();
  if (videoTracks.length > 0) {
    console.log(`Using video device: ${videoTracks[0].label}`);
  }
  if (audioTracks.length > 0) {
    console.log(`Using audio device: ${audioTracks[0].label}`);
  }
  const servers = [];
  let rtcPeer = new RTCPeerConnection(servers);
  
  console.log("Created local peer connection object rtcPeer");
  rtcPeer.onicecandidate = onIceCandidate;
  rtcPeer.ontrack = onRemoteTrack;
  rtcPeer.oniceconnectionstatechange = onIceConnectionStateChange;
  // Add media tracks to local peer
  localStream.getTracks()
        .forEach(track => rtcPeer.addTrack(track, localStream));
  console.log("Adding Local Stream to local peer connection");
  return rtcPeer;
}

function onRemoteTrack(event) {
  console.log("Remote track received: ", event);
  let clientID;
  for(let id of rtcPeers.keys()){
    if (rtcPeers.get(id) == this) {
      clientID = id;
      break;
    }
  }
  let remoteVideo;
  // TODO Search better solotion
  remoteVideo = document.getElementById(clientID);
  if (remoteVideo == null || remoteVideo == undefined) {
    addVideoBox(clientID);
  }
  console.log("remote streams: ", event.streams);
  remoteVideo.srcObject = event.streams[0];
}

async function onJoin(msg) {
  // create peer con with ID of peer
  console.log("Received join from: ", msg.ClientID);
  let peer = createPeerConnection();
  console.log("adding peer to map: ", msg.ClientID);
  rtcPeers.set(msg.ClientID, peer);
  addVideoBox(msg.ClientID);
  const offer = await createOffer(peer);
  await setOffer(peer, offer);
  console.log("sending offer: ", offer);
  ws.send(JSON.stringify({
    To: msg.ClientID,
    Data: {offer: offer}
  }));
}

async function onOffer(msg) {
  // create peer con with id of peer
  console.log("Received offer from: ", msg.ClientID);
  let peer = createPeerConnection();
  console.log("adding peer to map: ", msg.ClientID);
  rtcPeers.set(msg.ClientID, peer);
  addVideoBox(msg.ClientID);
  await addOffer(peer, msg.Data.offer);
  const answer = await createAnswer(peer);
  await addAnswer(peer, answer);
  console.log("sending answer to: ", msg.ClientID, answer);
  ws.send(JSON.stringify({
    To: msg.ClientID,
    Data: {answer: answer}
  }));
}

async function onAnswer(msg) {
  // add answer to peer with id
  let peer = rtcPeers.get(msg.ClientID);
  await setAnswer(peer, msg.Data.answer)
}

function onIceCandidate(event) {
  let clientID;
  // send ice candidate to peer with map[id] = this
  for(let id of rtcPeers.keys()){
    if (rtcPeers.get(id) == this) {
      clientID = id;
      break;
    }
  }
  console.log("Found icecandidate", event.candidate);
  if (event.candidate) {
    ws.send(JSON.stringify({
      To: clientID,
      Data: {iceCandidate: event.candidate}
    }));
  }
}

async function addIceCandidate(msg) {
  // add ice candidate to peer with id
  let peer = rtcPeers.get(msg.ClientID);
  try {
    await peer.addIceCandidate(msg.Data.iceCandidate);
    console.log("Added icecandidate", msg.Data.iceCandidate);
  } catch (e) {
    console.log(`Failed to add icecandidate: ${e.toString()}`);
  }
}

function onIceConnectionStateChange(event) {
  console.log("ICE state change event: ", this, event);
  // console.log("THIS ice conn: ", this);
  switch (this.iceConnectionState) {
    case "failed":
    case "disconnected":
    case "closed":
      // cleanup of map[id] = this
      cleanUp(this);
      break;
    default:
      break;
  }
}

async function createOffer(peer) {
  let offer;
  try {
    offer = await peer.createOffer(offerOptions);
    console.log("Created session description:", offer);
  } catch (e) {
    console.log(`Failed to create session description: ${e.toString()}`);
  }
  return offer;
}

async function setOffer(peer, offer) {
  try {
    await peer.setLocalDescription(offer);
    console.log("Set session description offer success.");
  } catch (e) {
    console.log(`Failed to set session offer description: ${e.toString()}`);
  }
}

async function addOffer(peer, offer) {
  try {
    await peer.setRemoteDescription(new RTCSessionDescription(offer));
    console.log("Add session description offer success.");
  } catch (e) {
    console.log(`Failed to add session offer description: ${e.toString()}`);
  }
}

async function createAnswer(peer) {
  let answer;
  try {
    answer = await peer.createAnswer();
    console.log("Created answer", answer);
  } catch (e) {
    console.log(`Failed to create answer: ${e.toString()}`);
  }
  return answer;
}

async function setAnswer(peer, answer) {
  try {
    await peer.setRemoteDescription(new RTCSessionDescription(answer));
    console.log("Set session description answer success.");
  } catch (e) {
    console.log(`Failed to set session answer description: ${e.toString()}`);
  }
}

async function addAnswer(peer, answer) {
  try {
    await peer.setLocalDescription(answer);
    console.log("Added local answer", answer);
  } catch (e) {
    console.log(`Failed to add answer: ${e.toString()}`);
  }
}

function addVideoBox(clientid) {
  console.log("Adding video box", clientid);
  let box = document.createElement("div");
  box.setAttribute("class", "video-box");
  let name = document.createElement("p");
  name.setAttribute("class", "clientName");
  name.innerText = clientid;
  box.appendChild(name);
  let video = document.createElement("video")
  video.setAttribute("id", clientid); 
  video.setAttribute("playsinline", "");
  video.setAttribute("autoplay", "");
  video.setAttribute("muted", "");
  box.appendChild(video);
  grid.appendChild(box);
}

async function remVideoBox(id) {
  console.log("Removing video box", id);
  let remoteVideo = document.getElementById(id);
  remoteVideo.srcObject.getTracks().forEach(track => track.stop());
  remoteVideo.parentElement.remove();
}

async function cleanUp(peer) {
  console.log("Cleanin up peer: ", peer);
  let clientID;
  for(let id of rtcPeers.keys()){
    if (rtcPeers.get(id) == peer) {
      remVideoBox(id);
      break;
    }
  }
  if (peer != null) {
    peer.close();
    peer = null;
  }
  rtcPeers.delete(clientID);
}

async function cleanUpID(id) {
  console.log("Cleanin up peer id: ", id);
  remVideoBox(id);
  let peer = rtcPeers.get(id)
  if (peer != null) {
    peer.close();
    peer = null;
  }
  rtcPeers.delete(id);
}

function cleanUpAll() {
  for(let id of rtcPeers.keys()) {
    let peer = rtcPeers.get(id);
    remVideoBox(id);
    peer.close();
    peer = null;
    rtcPeers.delete(id);
  }
}

function hangup() {
  console.log("Ending call");
  ws.send(JSON.stringify({Data: {hangup: true}}));
  cleanUpAll();
  if (ws != null) {
    ws.close(1000, "ending call");
    ws = null;
  }
  // HTTP redirect to main page
  window.location.replace(location.protocol+ "//" +location.host);
}

function randRoom() {
  let min = 1;
  let max = 100;
  let roomID = Math.floor(Math.random() * (max - min + 1) + min);
  let input = document.getElementById("RoomId");
  input.value = roomID;
}