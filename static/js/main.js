const grid = document.getElementById("video-grid");
const localVideo = document.getElementById('localVideo');

const startButton = document.getElementById('startButton');
const callButton = document.getElementById('callButton');
const hangupButton = document.getElementById('hangupButton');

let ws;
let localStream;
let localPeerConnection;

const offerOptions = {
  offerToReceiveAudio: 1,
  offerToReceiveVideo: 1
};

startButton.disabled = false;
callButton.disabled = true;
hangupButton.disabled = true;

startButton.addEventListener('click', start);
callButton.addEventListener('click', call);
hangupButton.addEventListener('click', hangup);

async function start() {
    console.log('Requesting local stream');
    startButton.disabled = true;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({audio: true, video: true});
      console.log('Received local stream');
      localVideo.srcObject = stream;
      localStream = stream;
      callButton.disabled = false;
    } catch (e) {
      alert(`getUserMedia() error: ${e.name}`);
    }
  }

async function cleanUp() {
  console.log("Cleaning up..");
  if (localPeerConnection != null) {
    localPeerConnection.close();
    localPeerConnection = null;
  }
  remVideoBox();
}

function hangup() {
  console.log('Ending call');
  cleanUp();
  if (ws != null) {
    ws.close(1000, "ending call");
    ws = null;
  }
  // HTTP redirect to main page
  window.location.replace(location.protocol+ "//" +location.host);
}
 
async function call() {
  callButton.disabled = true;
  hangupButton.disabled = false;
  let urlParams = new URLSearchParams(window.location.search);
  let roomID = urlParams.get("id");
  console.log(roomID);
  ws = new WebSocket("ws://" + location.host + "/ws?roomid=" +roomID);
  ws.onopen = wsOnOpen;
  ws.onclose = wsOnClose;
  ws.onmessage = wsOnMessage;
  ws.onerror = wsOnError;
}

function wsOnOpen (event) {
    console.log("trying to connect to ws")
    // Send request to connect to peers in room
    createPeerConnection();
    console.log("sending join");
    ws.send(JSON.stringify({join: true}));
  };

function wsOnClose (event) {
    if (event.wasClean) {
      console.log("ws conn closed clean.")
    } else {
      console.log("ws conn broken.", event);
    }
    console.log("code: " + event.code + " reason: " + event.reason);
  };

function wsOnMessage (event) {
  let msg = JSON.parse(event.data);
  // Join EVENT
  if(msg.Data.join) {
    console.log("getting mssage join: ", msg.Data.join);
    onJoin();
  // Offer received EVENT/ sdp
  } else if (msg.Data.offer){
    console.log("mssage offer: ", msg.Data.offer);
    onOffer(msg.Data.offer);
  // Answer received EVENT/ sdp
  } else if (msg.Data.answer) {
    console.log("getting answer: ", msg.Data.answer);
    onAnswer(msg.Data.answer);
  // Icecandidate EVENT
  } else if (msg.Data.iceCandidate) {
    console.log("getting mssage icecandidate: ", msg.Data.iceCandidate);
    addIceCandidate(msg.Data.iceCandidate);
  } else {
    console.log("Unknown message type: ", msg);
  }
};

function wsOnError(error) {
    console.log("ws error:", error.message);
};
//////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////

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
  localPeerConnection = new RTCPeerConnection(servers);
  
  console.log('Created local peer connection object localPeerConnection');
  localPeerConnection.onicecandidate = onIceCandidate;
  localPeerConnection.ontrack = onRemoteTrack;
  localPeerConnection.oniceconnectionstatechange = onIceConnectionStateChange;
  // localPeerConnection.onconnectionstatechange = onConnectionStateChange;
  // Add media tracks to local peer
  localStream.getTracks()
        .forEach(track => localPeerConnection.addTrack(track, localStream));
  console.log('Adding Local Stream to local peer connection');
}

function onRemoteTrack(event) {
  console.log("Remote track received: ", event);
  let remoteVideo;
  // TODO Search better solotion
  try {
    remoteVideo = document.getElementById("remote-0");
  }catch(e) {
    console.log(`Remote video null: ${e.toString()}`);
  }
  if (remoteVideo == null || remoteVideo == undefined) {
    addVideoBox();
  }
  console.log("remote streams: ", event.streams);
  remoteVideo.srcObject = event.streams[0];
}

async function onJoin() {
    const offer = await createOffer();
    await setOffer(offer);
    console.log("sending offer: ", offer);
    ws.send(JSON.stringify({offer: offer}));
}

async function onOffer(offer) {
    await addOffer(offer);
    const answer = await createAnswer();
    await addAnswer(answer);
    console.log("sending answer: ", answer);
    ws.send(JSON.stringify({answer: answer}));
}

async function onAnswer(answer) {
    await setAnswer(answer)
}

function onIceCandidate(event) {
  console.log("Found icecandidate", event.candidate);
  if (event.candidate) {
    ws.send(JSON.stringify({iceCandidate: event.candidate}));
  }
}

async function addIceCandidate(candidate) {
  try {
    await localPeerConnection.addIceCandidate(candidate);
    console.log("Added icecandidate", candidate);
  } catch (e) {
    console.log(`Failed to add icecandidate: ${e.toString()}`);
  }
}

function onIceConnectionStateChange(event) {
  console.log("ICE state change event: ", event);
  switch (localPeerConnection.iceConnectionState) {
    case "failed":
    case "disconnected":
    case "closed":
      cleanUp();
      break;
    default:
      break;
  }
}

async function createOffer() {
  let offer;
  try {
    offer = await localPeerConnection.createOffer(offerOptions);
    console.log("Created session description:", offer);
  } catch (e) {
    console.log(`Failed to create session description: ${e.toString()}`);
  }
  return offer;
}

async function setOffer(offer) {
  try {
    await localPeerConnection.setLocalDescription(offer);
    console.log('Set session description offer success.');
  } catch (e) {
    console.log(`Failed to set session offer description: ${e.toString()}`);
  }
}

async function addOffer(offer) {
  try {
    await localPeerConnection.setRemoteDescription(new RTCSessionDescription(offer));
    console.log('Add session description offer success.');
  } catch (e) {
    console.log(`Failed to add session offer description: ${e.toString()}`);
  }
}

async function createAnswer() {
  let answer;
  try {
    answer = await localPeerConnection.createAnswer();
    console.log('Created answer', answer);
  } catch (e) {
    console.log(`Failed to create answer: ${e.toString()}`);
  }
  return answer;
}

async function setAnswer(answer) {
  try {
    await localPeerConnection.setRemoteDescription(new RTCSessionDescription(answer));
    console.log('Set session description answer success.');
  } catch (e) {
    console.log(`Failed to set session answer description: ${e.toString()}`);
  }
}

async function addAnswer(answer) {
  try {
    await localPeerConnection.setLocalDescription(answer);
    console.log('Added local answer', answer);
  } catch (e) {
    console.log(`Failed to add answer: ${e.toString()}`);
  }
}

function addVideoBox() {
  console.log("Adding video box");
  let box = document.createElement("div");
  box.setAttribute("class", "video-box");
  let video = document.createElement("video")
  video.setAttribute("id", "remote-0"); 
  video.setAttribute("playsinline", "");
  video.setAttribute("autoplay", "");
  video.setAttribute("muted", "");
  box.appendChild(video);
  grid.appendChild(box);
}

async function remVideoBox() {
  console.log("Removing video box");
  let remoteVideo = document.getElementById("remote-0");
  remoteVideo.parentElement.remove();
}

// TODO
function randRoom() {}
