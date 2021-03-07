# CloudFort
CloudFort is a world save sharing program for the video game Dwarf Fortress for the purpose of facilitating community Dwarf Fortress games shared by many players.

# Quickstart Guide
1. Get the URL or IP address and port number for a CloudFort server.
2. Download CloudFort.exe (or CloudFort_Linux or CloudFort_Mac) and put it in your Dwarf Fortress game folder. ![step03](https://user-images.githubusercontent.com/1922739/110251242-c8e1cb80-7fd3-11eb-85d8-3dc8fedfa4e3.png)
3. Launch CloudFort.exe (by douple-clicking it)
4. Enter your overseer name and the server address in the pop-up menu ![step04](https://user-images.githubusercontent.com/1922739/110251263-dac36e80-7fd3-11eb-85cd-576b13a4f1d0.png)
5. Select an available world save from the menu ![step06](https://user-images.githubusercontent.com/1922739/110251292-f3cc1f80-7fd3-11eb-886f-934bfa3a6893.png)
6. Play Dwarf Fortress (Dwarf Fortress will automatically start after the shared save is downloaded)![step07](https://user-images.githubusercontent.com/1922739/110251316-0cd4d080-7fd4-11eb-9d82-ce9067e8010b.png)
![step08](https://user-images.githubusercontent.com/1922739/110251318-0f372a80-7fd4-11eb-96b3-592b8851b0aa.png)
7. When Dwarf Fortress exits, you will be asked to check the world back in.
![step10](https://user-images.githubusercontent.com/1922739/110251325-16f6cf00-7fd4-11eb-9bda-adfc1deb80cb.png)
8. Wait for the save to be uploaded. The command window will automatically close when the upload is finished.
![step11](https://user-images.githubusercontent.com/1922739/110251332-1c541980-7fd4-11eb-9509-96f10bf34350.png)
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

# How does CloudFort work?
CloudFort is a two-part server-client program.

On the server side, CloudFort-Server creates a save folder, which it scans on start-up to detect any new zipped world saves. For each .zip file containing a Dwarf Fortress save, CloudFort-Server creates a .dftk token file (containing JSON data) to track the file's state. There are three states: _available_, _downloading_, and _checked-out_. Clients can request any _available_ save. Upon receiving such a request, CloudFort-Server changes it's status to _downloading_, locking that world for up to the download time limit (default 30 minutes) for the client to download the zip file. If sucessfully downloaded, the client is given a unique "magic rune sequence" that ensures that only they can check it back in and the status is changed to _checked-out_ for a period of time (default 8 hours). The client has that much time to play the world and then check it back in. A save can only be checked-in by the client who checked it out, and only if the status on the server side is still _checked-out_. If the client fails to check-in (or download) within the alloted time, the save reverts back to its pre-check-out state and becomes _available_ again.

On the client side, CloudFort connect to the server, then requests the status of all worlds. If the user selects an available world, it is downloaded and checked out, then extracted into the Dwarf Fortress save folder. It then launches the Dwarf Fortress executable, so you can play Dwarf Fortress and have the downloaded save available to play. When you quit Dwarf Fortress (or when you start CloudFort again with a checked-out save in your DF save folder), you will be asked if you want to check the saved world back in. If you answer "no", then you can either can simply leave it checked-out to return to later or request that the server revert this world backto the way it was before you checked it out. When the user does decide to check-in the save, CloudFort packages it up as a .zip file and uploads it to the server, presenting a "magic rune sequence" to validate the save. After the upload is complete, CloudFort exits. 

Configuration details for CloudFort and CloudFort-Server are stored in .json files (_CloudFort-config.json_ and _server-config.json_, respectively).
