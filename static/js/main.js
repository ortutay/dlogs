$(document).init(function() {
  var logs = new WebSocket('ws://' + location.host + '/logs');
  logs.onmessage = handleLogsStreamMessage;
});

function handleLogsStreamMessage(e) {
  $("#logs").append("<div class='log-line'>" + e.data + "</div>");
  if ($("#logs").height() > window.innerHeight) {
    $("#logs").css("position", "absolute");
    $("#logs").css("bottom", 0);
  }
}
