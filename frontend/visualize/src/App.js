import React, { useState } from 'react';
import logo from './logo.svg';
import './App.css';
import MyDatePicker from './MyDatePicker';

function App() {
  const [mcapDateFiles, setMcapDateFiles] = useState([]);
  const [matDateFiles, setMatDateFiles] = useState([]);
  const serverAddress = 'http://127.0.0.1:5000';
  const [selectedDate, setSelectedDate] = useState(null);

  const handleDateChange = (date) => {
    setSelectedDate(date);
  };
  const seeAll = async () => {
    try {

      const response = await fetch(`${serverAddress}/get_runs`, {
        method: 'POST',
      });
  
      if (!response.ok) {
        throw new Error('Failed to fetch runs');
      }
      
      const runsData = await response.json();
  
      const mcapFiles = [];
      const matFiles = [];
      
      for (let index = 0; index < runsData.length; index++) {
        const element = runsData[index];
        mcapFiles.push(element.mcap_download_link);
        matFiles.push(element.matlab_download_link);
      }
      
      setMcapDateFiles(mcapFiles);
      setMatDateFiles(matFiles);
    } catch (error) {
      console.error('Error fetching runs:', error.message);
      // Handle error (e.g., show error message to the user)
    }
  }
  
  const handleButtonClick = async () => {
    if (!selectedDate) {
      console.error('No date selected.');
      return;
    }
    
    const formattedDate = `${(selectedDate.getMonth() + 1).toString().padStart(2, '0')}-${selectedDate.getDate().toString().padStart(2, '0')}-${selectedDate.getFullYear()}`;
    console.log('Selected date:', formattedDate);

    
    try {
      const formData = new FormData();
      formData.append('date', formattedDate);

      const response = await fetch(`${serverAddress}/get_runs`, {
        method: 'POST',
        body: formData,
      });
  
      if (!response.ok) {
        throw new Error('Failed to fetch runs');
      }
      
      const runsData = await response.json();
  
      const mcapFiles = [];
      const matFiles = [];
      
      for (let index = 0; index < runsData.length; index++) {
        const element = runsData[index];
        mcapFiles.push(element.mcap_download_link);
        matFiles.push(element.matlab_download_link);
      }
      
      setMcapDateFiles(mcapFiles);
      setMatDateFiles(matFiles);
    } catch (error) {
      console.error('Error fetching runs:', error.message);
      // Handle error (e.g., show error message to the user)
    }
    
    //TESTING WITH LINKS CUZ RN BACKEND IS NOT WORKING
    //Uncomment top and delete bottom

    /** 
    const mcapFiles = [];
    const matFiles = [];
    mcapFiles.push("https://run-metadata.s3.amazonaws.com/03-26-2024/03_05_2024_23_10_23_V123.mcap?AWSAccessKeyId=AKIA4HTQXVTTEN6PEIUS&Signature=yDv3whWWO%2FtXmmQZhP3XwiIGT30%3D&Expires=1712890495");
    matFiles.push("https://run-metadata.s3.amazonaws.com/03-26-2024/03_05_2024_23_10_23_V123.mat?AWSAccessKeyId=AKIA4HTQXVTTEN6PEIUS&Signature=ofvYjdbMaekGQvWsHzXk6siUsyc%3D&Expires=1712890515");
    setMcapDateFiles(mcapFiles);
    setMatDateFiles(matFiles);
    */
  };
  
  // Function to extract substring from the URL
  const getLinkName = (link) => {
    return link.substring(39, 48); // Extract characters from index 39 to 48
  };

  return (
    <div className="App">
      <MyDatePicker selectedDate={selectedDate} onDateChange={handleDateChange} />
      <button onClick={handleButtonClick}>Get Selected Date</button>
      <button onClick={seeAll}>See All</button>
      <br></br>
      <div className='ParentTable'>
        {/* Render MCAP and MATLAB download links */}
        <div className='McapChild'>
          <h3>MCAP Files:</h3>
          {mcapDateFiles.map((link, index) => (
            <a key={index} href={link} target="_blank" rel="noopener noreferrer">{getLinkName(link)}</a> // Change here
          ))}
        </div>
        <div className='MatChild'>
          <h3>MATLAB Files:</h3>
          {matDateFiles.map((link, index) => (
            <a key={index} href={link} target="_blank" rel="noopener noreferrer">{getLinkName(link)}</a> // Change here
          ))}
        </div>
      </div>
    </div>
  );
}

export default App;