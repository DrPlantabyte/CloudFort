# CloudFort
CloudFort is a world save sharing program for the video game Dwarf Fortress for the purpose of facilitating community Dwarf Fortress games shared by many players.

# Quickstart Guide
1. Get the URL or IP address and port number for a CloudFort server.
2. Download CloudFort.exe (or CloudFort_Linux or CloudFort_Mac) and put it in your Dwarf Fortress game folder.
3. Launch CloudFort.exe (by douple-clicking it)
4. Enter your overseer name and the server address in the pop-up menu
5. Select an available world save from the menu
6. Play Dwarf Fortress (Dwarf Fortress will automatically start after the shared save is downloaded)
7. When Dwarf Fortress exits, you will be asked to check the world back in.
8. Wait for the save to be uploaded. The command window will automatically close when the upload is finished.
9. Tell your friends about your Dwarf Fortress and encourange them to check-out the same world save to continue where you left off.

# Installation
Installing CloudFort is as easy as dropping the CloudFort.exe file (or CloudFort_Linux or CloudFort_Mac) in your Dwarf Fortress folder and double-clicking on it. You will need the IP address and port number of a CloudFort server to use CloudFort.

# CloudFort Server Setup
To run a CloudFort server, simply run CloudFort-Server.exe (or CloudFort-Server_Linux or CloudFort-Server_Mac) in whatever folder you want to act as the filestore for the shared world saves. Edit **server-config.json** to change the server default settings.
## Added a Dwarf Fortress save
1. Zip the save folder as a .zip file.
2. Copy the .zip folder to the server's save folder
3. Restart the server

## World Save Management
Any number of saves can be added to the server, but each can only be checked-out by one player at a time (called an overseer). When a player checks-out a save, it is locked until it is checked back in or the check-out time expires (default checkout time limit is 8 hours). If you need to manually un-checkout a world, you must stop the server program, then delete the save's .dftk file, then start the server again.
